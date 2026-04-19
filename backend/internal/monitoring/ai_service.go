package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"
)

// AIService orchestrates AI-powered incident analysis.
// It manages the analysis lifecycle: queue → claim → analyze → complete/fail.
type AIService struct {
	mu sync.RWMutex

	configStore  *AIConfigStore
	aiQueue      *FileAIQueue
	incidentRepo IncidentRepository
	snapshotRepo IncidentSnapshotRepository
	store        Store // for check results
	logger       *log.Logger

	// runtime state
	providers map[string]AIProvider // id -> provider
	stopCh    chan struct{}
	running   bool
}

// NewAIService creates the AI orchestrator.
func NewAIService(
	configStore *AIConfigStore,
	aiQueue *FileAIQueue,
	incidentRepo IncidentRepository,
	snapshotRepo IncidentSnapshotRepository,
	store Store,
	logger *log.Logger,
) *AIService {
	if logger == nil {
		logger = log.Default()
	}

	svc := &AIService{
		configStore:  configStore,
		aiQueue:      aiQueue,
		incidentRepo: incidentRepo,
		snapshotRepo: snapshotRepo,
		store:        store,
		logger:       logger,
		providers:    make(map[string]AIProvider),
		stopCh:       make(chan struct{}),
	}

	// Build providers from config
	svc.rebuildProviders()

	return svc
}

// rebuildProviders recreates provider instances from config.
func (s *AIService) rebuildProviders() {
	cfg := s.configStore.Get()
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second

	newProviders := make(map[string]AIProvider)
	for _, pc := range cfg.Providers {
		if !pc.Enabled {
			continue
		}
		provider, err := NewAIProvider(pc, timeout)
		if err != nil {
			s.logger.Printf("AI: failed to create provider %s: %v", pc.ID, err)
			continue
		}
		newProviders[pc.ID] = provider
	}

	s.mu.Lock()
	s.providers = newProviders
	s.mu.Unlock()

	s.logger.Printf("AI: rebuilt %d active providers", len(newProviders))
}

// getDefaultProvider returns the default (or first enabled) provider.
func (s *AIService) getDefaultProvider() (AIProvider, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.configStore.Get()

	// Find explicit default
	for _, pc := range cfg.Providers {
		if pc.IsDefault && pc.Enabled {
			if p, ok := s.providers[pc.ID]; ok {
				return p, pc.ID, nil
			}
		}
	}

	// Fall back to first enabled
	for _, pc := range cfg.Providers {
		if pc.Enabled {
			if p, ok := s.providers[pc.ID]; ok {
				return p, pc.ID, nil
			}
		}
	}

	return nil, "", fmt.Errorf("no enabled AI provider configured")
}

// getProvider returns a specific provider by ID.
func (s *AIService) getProvider(providerID string) (AIProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("provider %q not found or not enabled", providerID)
	}
	return p, nil
}

// --- Queue Processing ---

// StartWorker begins background processing of the AI queue.
func (s *AIService) StartWorker() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.workerLoop()
	s.logger.Printf("AI: worker started")
}

// StopWorker stops background processing.
func (s *AIService) StopWorker() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.logger.Printf("AI: worker stopped")
}

func (s *AIService) workerLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			cfg := s.configStore.Get()
			if !cfg.Enabled {
				continue
			}
			s.processQueue(cfg)
		}
	}
}

func (s *AIService) processQueue(cfg AIServiceConfig) {
	items, err := s.aiQueue.ClaimPending(cfg.MaxConcurrent)
	if err != nil {
		s.logger.Printf("AI: failed to claim pending items: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, cfg.MaxConcurrent)

	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore

		go func(item AIQueueItem) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			s.analyzeItem(item, cfg)
		}(item)
	}

	wg.Wait()
}

func (s *AIService) analyzeItem(item AIQueueItem, cfg AIServiceConfig) {
	provider, providerID, err := s.getDefaultProvider()
	if err != nil {
		_ = s.aiQueue.Fail(item.IncidentID, fmt.Sprintf("no provider: %v", err))
		return
	}

	// Build the prompt
	systemMsg, userMsg, err := s.buildPrompt(item, cfg)
	if err != nil {
		_ = s.aiQueue.Fail(item.IncidentID, fmt.Sprintf("build prompt: %v", err))
		return
	}

	// Call AI with retry
	var responseText string
	var lastErr error
	maxAttempts := cfg.RetryCount + 1

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(cfg.RetryDelayMs) * time.Millisecond)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
		responseText, lastErr = provider.Analyze(ctx, systemMsg, userMsg)
		cancel()

		if lastErr == nil {
			break
		}

		s.logger.Printf("AI: attempt %d/%d failed for incident %s (provider %s): %v",
			attempt+1, maxAttempts, item.IncidentID, providerID, lastErr)
	}

	if lastErr != nil {
		_ = s.aiQueue.Fail(item.IncidentID, fmt.Sprintf("all %d attempts failed: %v", maxAttempts, lastErr))
		return
	}

	// Parse the AI response into structured result
	result := s.parseAnalysisResponse(item.IncidentID, responseText)

	if err := s.aiQueue.Complete(item.IncidentID, result); err != nil {
		s.logger.Printf("AI: failed to complete item %s: %v", item.IncidentID, err)
		return
	}

	s.logger.Printf("AI: analysis completed for incident %s (provider: %s)", item.IncidentID, providerID)
}

// --- Prompt Building ---

func (s *AIService) buildPrompt(item AIQueueItem, cfg AIServiceConfig) (string, string, error) {
	// Find the right prompt template
	tmpl := s.findPromptTemplate(item, cfg)

	// Gather context about the incident
	incident, err := s.incidentRepo.GetIncident(item.IncidentID)
	if err != nil {
		return "", "", fmt.Errorf("get incident: %w", err)
	}

	// Gather evidence snapshots
	var evidence string
	if s.snapshotRepo != nil {
		snapshots, err := s.snapshotRepo.GetSnapshots(item.IncidentID)
		if err == nil && len(snapshots) > 0 {
			var parts []string
			for _, snap := range snapshots {
				parts = append(parts, fmt.Sprintf("--- %s ---\n%s", snap.SnapshotType, snap.PayloadJSON))
			}
			evidence = strings.Join(parts, "\n\n")
		}
	}

	// Gather recent check results
	var recentResults string
	if s.store != nil {
		state := s.store.Snapshot()
		var matchingResults []CheckResult
		for _, r := range state.Results {
			if r.CheckID == incident.CheckID {
				matchingResults = append(matchingResults, r)
			}
		}
		// Last 10 results
		if len(matchingResults) > 10 {
			matchingResults = matchingResults[len(matchingResults)-10:]
		}
		if len(matchingResults) > 0 {
			resultsJSON, _ := json.MarshalIndent(matchingResults, "", "  ")
			recentResults = string(resultsJSON)
		}
	}

	// Build template data
	data := map[string]interface{}{
		"IncidentID":    incident.ID,
		"CheckName":     incident.CheckName,
		"CheckType":     incident.Type,
		"Severity":      incident.Severity,
		"Message":       incident.Message,
		"StartedAt":     incident.StartedAt.Format(time.RFC3339),
		"Duration":      time.Since(incident.StartedAt).Round(time.Second).String(),
		"Evidence":      evidence,
		"RecentResults": recentResults,
		"RuleCode":      incident.Metadata["ruleId"],
	}

	// Add MySQL-specific fields if present in evidence
	if evidence != "" {
		data["LatestSample"] = extractSnapshot(evidence, "latest_sample")
		data["RecentDeltas"] = extractSnapshot(evidence, "recent_deltas")
		data["ProcessList"] = extractSnapshot(evidence, "processlist")
		data["StatementAnalysis"] = extractSnapshot(evidence, "statement_analysis")
	}

	systemMsg, err := renderTemplate(tmpl.SystemMsg, data)
	if err != nil {
		return "", "", fmt.Errorf("render system message: %w", err)
	}

	userMsg, err := renderTemplate(tmpl.UserMsg, data)
	if err != nil {
		return "", "", fmt.Errorf("render user message: %w", err)
	}

	return systemMsg, userMsg, nil
}

func (s *AIService) findPromptTemplate(item AIQueueItem, cfg AIServiceConfig) AIPromptTemplate {
	// Check if there's a specific prompt version requested
	if item.PromptVersion != "" {
		for _, p := range cfg.Prompts {
			if p.ID == item.PromptVersion || p.Version == item.PromptVersion {
				return p
			}
		}
	}

	// Use configured default prompt
	if cfg.DefaultPromptID != "" {
		for _, p := range cfg.Prompts {
			if p.ID == cfg.DefaultPromptID {
				return p
			}
		}
	}

	// Fall back to the prompt marked as default
	for _, p := range cfg.Prompts {
		if p.IsDefault {
			return p
		}
	}

	// Absolute fallback
	defaults := defaultPromptTemplates()
	return defaults[0]
}

func extractSnapshot(evidence, snapshotType string) string {
	marker := fmt.Sprintf("--- %s ---", snapshotType)
	idx := strings.Index(evidence, marker)
	if idx < 0 {
		return "(not available)"
	}

	content := evidence[idx+len(marker):]
	// Find the next snapshot marker
	nextIdx := strings.Index(content, "\n--- ")
	if nextIdx >= 0 {
		content = content[:nextIdx]
	}

	return strings.TrimSpace(content)
}

func renderTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	t, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// --- Response Parsing ---

func (s *AIService) parseAnalysisResponse(incidentID, responseText string) AIAnalysisResult {
	result := AIAnalysisResult{
		IncidentID: incidentID,
		Analysis:   responseText,
		CreatedAt:  time.Now().UTC(),
	}

	// Try to parse structured JSON from the response
	// The AI might return JSON directly or embed it in markdown code blocks
	jsonStr := extractJSON(responseText)
	if jsonStr != "" {
		var parsed struct {
			RootCause            string      `json:"rootCause"`
			Impact               string      `json:"impact"`
			Severity             string      `json:"severity"`
			Suggestions          []string    `json:"suggestions"`
			Confidence           string      `json:"confidence"`
			AdditionalDataNeeded []string    `json:"additionalDataNeeded"`
			Urgency              string      `json:"urgency"`
			MySQLSpecific        interface{} `json:"mysqlSpecific"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			// Build a clean analysis string
			var analysis strings.Builder
			if parsed.RootCause != "" {
				analysis.WriteString("**Root Cause**: " + parsed.RootCause + "\n\n")
			}
			if parsed.Impact != "" {
				analysis.WriteString("**Impact**: " + parsed.Impact + "\n\n")
			}
			if parsed.Confidence != "" {
				analysis.WriteString("**Confidence**: " + parsed.Confidence + "\n\n")
			}
			if parsed.Urgency != "" {
				analysis.WriteString("**Urgency**: " + parsed.Urgency + "\n\n")
			}
			if parsed.MySQLSpecific != nil {
				mysqlJSON, _ := json.MarshalIndent(parsed.MySQLSpecific, "", "  ")
				analysis.WriteString("**MySQL Analysis**:\n```json\n" + string(mysqlJSON) + "\n```\n\n")
			}
			if len(parsed.AdditionalDataNeeded) > 0 {
				analysis.WriteString("**Additional Data Needed**:\n")
				for _, d := range parsed.AdditionalDataNeeded {
					analysis.WriteString("- " + d + "\n")
				}
			}

			if analysis.Len() > 0 {
				result.Analysis = analysis.String()
			}
			if len(parsed.Suggestions) > 0 {
				result.Suggestions = parsed.Suggestions
			}
			if parsed.Severity != "" {
				result.Severity = parsed.Severity
			}
		}
	}

	return result
}

// extractJSON extracts JSON from a response that might be wrapped in markdown code blocks.
func extractJSON(text string) string {
	// Try direct parse first
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		return text
	}

	// Try extracting from ```json ... ``` blocks
	jsonStart := strings.Index(text, "```json")
	if jsonStart >= 0 {
		text = text[jsonStart+7:]
		jsonEnd := strings.Index(text, "```")
		if jsonEnd >= 0 {
			return strings.TrimSpace(text[:jsonEnd])
		}
	}

	// Try extracting from ``` ... ``` blocks
	codeStart := strings.Index(text, "```")
	if codeStart >= 0 {
		text = text[codeStart+3:]
		codeEnd := strings.Index(text, "```")
		if codeEnd >= 0 {
			candidate := strings.TrimSpace(text[:codeEnd])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	return ""
}

// --- Manual Analysis Trigger ---

// AnalyzeIncident triggers AI analysis for a specific incident (on-demand, not queued).
func (s *AIService) AnalyzeIncident(ctx context.Context, incidentID string, providerID string) (*AIAnalysisResult, error) {
	cfg := s.configStore.Get()
	if !cfg.Enabled {
		return nil, fmt.Errorf("AI analysis is disabled")
	}

	var provider AIProvider
	var err error

	if providerID != "" {
		provider, err = s.getProvider(providerID)
		if err != nil {
			return nil, err
		}
	} else {
		provider, providerID, err = s.getDefaultProvider()
		if err != nil {
			return nil, err
		}
	}

	item := AIQueueItem{
		IncidentID:    incidentID,
		PromptVersion: cfg.DefaultPromptID,
	}

	systemMsg, userMsg, err := s.buildPrompt(item, cfg)
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	responseText, err := provider.Analyze(ctx, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", providerID, err)
	}

	result := s.parseAnalysisResponse(incidentID, responseText)

	// Store the result
	if err := s.aiQueue.Complete(incidentID, result); err != nil {
		s.logger.Printf("AI: completed analysis but failed to persist for %s: %v", incidentID, err)
	}

	return &result, nil
}

// EnqueueIncidentAnalysis adds an incident to the AI analysis queue.
func (s *AIService) EnqueueIncidentAnalysis(incidentID string) error {
	cfg := s.configStore.Get()
	if !cfg.Enabled || !cfg.AutoAnalyze {
		return nil // silently skip if AI is disabled
	}

	promptVersion := cfg.DefaultPromptID
	if promptVersion == "" {
		promptVersion = "v1"
	}

	return s.aiQueue.Enqueue(incidentID, promptVersion)
}

// --- Provider Health Check ---

// ProviderHealth holds the health status of a provider.
type ProviderHealth struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Healthy   bool   `json:"healthy"`
	IsDefault bool   `json:"isDefault"`
}

// CheckProviderHealth tests connectivity to all configured providers.
func (s *AIService) CheckProviderHealth(ctx context.Context) []ProviderHealth {
	cfg := s.configStore.Get()
	results := make([]ProviderHealth, 0, len(cfg.Providers))

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, pc := range cfg.Providers {
		if !pc.Enabled {
			continue
		}

		health := ProviderHealth{
			ID:        pc.ID,
			Provider:  string(pc.Provider),
			Model:     pc.Model,
			IsDefault: pc.IsDefault,
		}

		if p, ok := s.providers[pc.ID]; ok {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			health.Healthy = p.IsHealthy(checkCtx)
			cancel()
		}

		results = append(results, health)
	}

	return results
}

// --- Config Reload ---

// ReloadProviders rebuilds providers from updated config.
func (s *AIService) ReloadProviders() {
	s.rebuildProviders()
}
