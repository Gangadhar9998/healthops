package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

// --- AI Config Tests ---

func TestAIConfigDefaults(t *testing.T) {
	store := newMemoryAIConfigStore()
	cfg := store.Get()
	if cfg.Enabled {
		t.Error("expected disabled by default")
	}
	if !cfg.AutoAnalyze {
		t.Error("expected autoAnalyze=true by default")
	}
	if cfg.MaxConcurrent != 2 {
		t.Errorf("expected maxConcurrent=2, got %d", cfg.MaxConcurrent)
	}
	if len(cfg.Prompts) == 0 {
		t.Error("expected default prompts")
	}
}

func TestMemoryAIConfigStoreUpdate(t *testing.T) {
	store := newMemoryAIConfigStore()

	err := store.Update(func(cfg *AIServiceConfig) error {
		cfg.Enabled = true
		cfg.MaxConcurrent = 5
		cfg.Providers = append(cfg.Providers, AIProviderConfig{
			ID:       "test-openai",
			Provider: AIProviderOpenAI,
			Name:     "Test OpenAI",
			APIKey:   "sk-test-key-1234567890abcdef",
			Model:    "gpt-4o",
			Enabled:  true,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	cfg := store.Get()
	if !cfg.Enabled {
		t.Error("expected enabled after update")
	}
	if cfg.MaxConcurrent != 5 {
		t.Errorf("expected maxConcurrent=5, got %d", cfg.MaxConcurrent)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].APIKey != "sk-test-key-1234567890abcdef" {
		t.Errorf("API key not retained correctly: %s", cfg.Providers[0].APIKey)
	}
}

func TestAIConfigStoreMaskAPIKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sk-test1234567890abcdef", "sk-t...****cdef"},
		{"short", "****"},
		{"", ""},
		{"12345678", "****"},
	}

	for _, tt := range tests {
		got := maskAPIKey(tt.input)
		if got != tt.expected {
			t.Errorf("maskAPIKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestAIConfigStoreSafeView(t *testing.T) {
	store := newMemoryAIConfigStore()

	err := store.Update(func(cfg *AIServiceConfig) error {
		cfg.Providers = append(cfg.Providers, AIProviderConfig{
			ID:       "test-openai",
			Provider: AIProviderOpenAI,
			Name:     "Test",
			APIKey:   "sk-verysecretkey1234567890",
			Model:    "gpt-4o",
			Enabled:  true,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	cfg := store.Get()
	safe := cfg.toSafeView()

	if len(safe.Providers) != 1 {
		t.Fatalf("expected 1 provider in safe view")
	}

	// Ensure API key is masked
	if strings.Contains(safe.Providers[0].APIKey, "verysecret") {
		t.Error("API key should be masked in safe view")
	}
	if safe.Providers[0].APIKey == "" {
		t.Error("API key mask should not be empty")
	}
}

func TestAIConfigStoreValidation(t *testing.T) {
	store := newMemoryAIConfigStore()

	// Invalid: missing model
	err := store.Update(func(cfg *AIServiceConfig) error {
		cfg.Providers = append(cfg.Providers, AIProviderConfig{
			ID:       "bad",
			Provider: AIProviderOpenAI,
			Name:     "Bad",
			APIKey:   "sk-test",
			Model:    "", // missing
			Enabled:  true,
		})
		return cfg.validate()
	})
	if err == nil {
		t.Error("expected validation error for missing model")
	}
}

// --- AI Provider Factory Test ---

func TestNewAIProviderFactory(t *testing.T) {
	tests := []struct {
		name     string
		config   AIProviderConfig
		wantName string
		wantErr  bool
	}{
		{
			name: "openai",
			config: AIProviderConfig{
				Provider: AIProviderOpenAI,
				Name:     "OpenAI",
				APIKey:   "sk-test",
				Model:    "gpt-4o",
			},
			wantName: "openai:gpt-4o",
		},
		{
			name: "anthropic",
			config: AIProviderConfig{
				Provider: AIProviderAnthropic,
				Name:     "Anthropic",
				APIKey:   "sk-ant-test",
				Model:    "claude-3-5-sonnet",
			},
			wantName: "anthropic:claude-3-5-sonnet",
		},
		{
			name: "google",
			config: AIProviderConfig{
				Provider: AIProviderGoogle,
				Name:     "Gemini",
				APIKey:   "AIza-test",
				Model:    "gemini-2.0-flash",
			},
			wantName: "google:gemini-2.0-flash",
		},
		{
			name: "ollama",
			config: AIProviderConfig{
				Provider: AIProviderOllama,
				Name:     "Local Ollama",
				Model:    "llama3",
				BaseURL:  "http://localhost:11434",
			},
			wantName: "ollama:llama3",
		},
		{
			name: "custom openai-compatible",
			config: AIProviderConfig{
				Provider: AIProviderCustom,
				Name:     "Custom LLM",
				APIKey:   "custom-key",
				Model:    "my-model",
				BaseURL:  "http://my-llm:8080",
			},
			wantName: "openai:my-model",
		},
		{
			name: "unknown provider",
			config: AIProviderConfig{
				Provider: AIProviderType("invalid"),
				Name:     "Bad",
				Model:    "m",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewAIProvider(tt.config, 30*time.Second)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Name() != tt.wantName {
				t.Errorf("got name %q, want %q", p.Name(), tt.wantName)
			}
		})
	}
}

// --- AI Service Tests ---

func TestAIServiceEnqueueAnalysis(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	aiQueue := newMemoryAIQueue()

	svc := NewAIService(configStore, aiQueue, nil, nil, nil, nil)

	// With AI disabled, enqueue should silently succeed
	err := svc.EnqueueIncidentAnalysis("inc-123")
	if err != nil {
		t.Fatalf("enqueue with disabled AI: %v", err)
	}

	// Check nothing was enqueued
	items := aiQueue.AllItems()
	if len(items) != 0 {
		t.Errorf("expected 0 items when disabled, got %d", len(items))
	}

	// Enable AI
	_ = configStore.Update(func(cfg *AIServiceConfig) error {
		cfg.Enabled = true
		cfg.AutoAnalyze = true
		return nil
	})

	err = svc.EnqueueIncidentAnalysis("inc-456")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	items = aiQueue.AllItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].IncidentID != "inc-456" {
		t.Errorf("expected incident ID inc-456, got %s", items[0].IncidentID)
	}
}

func TestAIServiceNoProviderError(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	_ = configStore.Update(func(cfg *AIServiceConfig) error {
		cfg.Enabled = true
		return nil
	})

	aiQueue := newMemoryAIQueue()
	svc := NewAIService(configStore, aiQueue, nil, nil, nil, nil)

	_, err := svc.AnalyzeIncident(context.Background(), "inc-789", "")
	if err == nil {
		t.Error("expected error with no provider")
	}
	if !strings.Contains(err.Error(), "no enabled AI provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAIServiceGetResults(t *testing.T) {
	aiQueue := newMemoryAIQueue()

	// Complete an item with a result
	_ = aiQueue.Enqueue("inc-result-1", "v1")
	_, _ = aiQueue.ClaimPending(1)
	_ = aiQueue.Complete("inc-result-1", monitoring.AIAnalysisResult{
		IncidentID:  "inc-result-1",
		Analysis:    "Root cause: disk full",
		Suggestions: []string{"expand disk", "cleanup old files"},
		CreatedAt:   time.Now().UTC(),
	})

	results := aiQueue.GetResults("inc-result-1")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Analysis != "Root cause: disk full" {
		t.Errorf("unexpected analysis: %s", results[0].Analysis)
	}
	if len(results[0].Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(results[0].Suggestions))
	}
}

// --- Prompt Template Tests ---

func TestDefaultPromptTemplates(t *testing.T) {
	prompts := defaultPromptTemplates()
	if len(prompts) < 2 {
		t.Fatalf("expected at least 2 default prompts, got %d", len(prompts))
	}

	// Check incident-analysis-v1 is default
	found := false
	for _, p := range prompts {
		if p.ID == "incident-analysis-v1" && p.IsDefault {
			found = true
		}
	}
	if !found {
		t.Error("expected incident-analysis-v1 to be default prompt")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool // whether JSON is extracted
	}{
		{
			name:  "direct JSON",
			input: `{"rootCause": "disk full"}`,
			want:  true,
		},
		{
			name:  "markdown code block",
			input: "Here is my analysis:\n```json\n{\"rootCause\": \"disk full\"}\n```\n",
			want:  true,
		},
		{
			name:  "plain text",
			input: "The root cause appears to be a disk full condition.",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			if tt.want && result == "" {
				t.Error("expected JSON to be extracted")
			}
			if !tt.want && result != "" {
				t.Errorf("expected no JSON, got: %s", result)
			}
		})
	}
}

func TestParseAnalysisResponse(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	aiQueue := newMemoryAIQueue()
	svc := NewAIService(configStore, aiQueue, nil, nil, nil, nil)

	// Test structured response
	response := `{"rootCause": "Memory leak in connection pool", "impact": "Service degradation", "severity": "critical", "suggestions": ["Restart service", "Tune pool size"], "confidence": "high"}`
	result := svc.parseAnalysisResponse("inc-test", response)

	if result.IncidentID != "inc-test" {
		t.Errorf("expected incident ID inc-test, got %s", result.IncidentID)
	}
	if len(result.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(result.Suggestions))
	}
	if result.Severity != "critical" {
		t.Errorf("expected severity critical, got %s", result.Severity)
	}
}

// --- AI API Handler Tests ---

func TestAIAPIConfigGet(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	handler := NewAIAPIHandler(nil, configStore, nil, &monitoring.Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/config", nil)
	rr := httptest.NewRecorder()
	handler.handleAIConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp monitoring.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Error("expected ok=true")
	}
}

func TestAIAPIConfigUpdate(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	handler := NewAIAPIHandler(nil, configStore, nil, &monitoring.Config{})

	body := `{"enabled": true, "maxConcurrent": 4}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ai/config", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.handleAIConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	cfg := configStore.Get()
	if !cfg.Enabled {
		t.Error("expected enabled after update")
	}
	if cfg.MaxConcurrent != 4 {
		t.Errorf("expected maxConcurrent=4, got %d", cfg.MaxConcurrent)
	}
}

func TestAIAPIProvidersCRUD(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	aiQueue := newMemoryAIQueue()
	aiService := NewAIService(configStore, aiQueue, nil, nil, nil, nil)
	handler := NewAIAPIHandler(aiService, configStore, nil, &monitoring.Config{})

	// CREATE
	body := `{"id":"p1","provider":"openai","name":"My OpenAI","apiKey":"sk-test1234567890abcdef","model":"gpt-4o","enabled":true,"isDefault":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/providers", strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.handleAIProviders(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify key is masked in response
	if strings.Contains(rr.Body.String(), "sk-test1234567890abcdef") {
		t.Error("API key should be masked in response")
	}

	// LIST
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/providers", nil)
	rr = httptest.NewRecorder()
	handler.handleAIProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	// UPDATE
	body = `{"provider":"openai","name":"Updated OpenAI","apiKey":"sk-test1234567890abcdef","model":"gpt-4o-mini","enabled":true}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/ai/providers/p1", strings.NewReader(body))
	rr = httptest.NewRecorder()
	handler.handleAIProviderByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify model updated
	cfg := configStore.Get()
	for _, p := range cfg.Providers {
		if p.ID == "p1" {
			if p.Model != "gpt-4o-mini" {
				t.Errorf("expected model gpt-4o-mini, got %s", p.Model)
			}
			// Verify API key was preserved (sent empty in update)
			if p.APIKey == "" {
				t.Error("API key should be preserved when not sent in update")
			}
		}
	}

	// DELETE
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/ai/providers/p1", nil)
	rr = httptest.NewRecorder()
	handler.handleAIProviderByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	cfg = configStore.Get()
	if len(cfg.Providers) != 0 {
		t.Errorf("expected 0 providers after delete, got %d", len(cfg.Providers))
	}
}

func TestAIAPIPromptsCRUD(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	handler := NewAIAPIHandler(nil, configStore, nil, &monitoring.Config{})

	// LIST defaults
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/prompts", nil)
	rr := httptest.NewRecorder()
	handler.handleAIPrompts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	// CREATE
	body := `{"id":"custom-1","name":"Custom Prompt","systemMsg":"You are a test bot","userMsg":"Analyze: {{.Message}}","version":"v1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/ai/prompts", strings.NewReader(body))
	rr = httptest.NewRecorder()
	handler.handleAIPrompts(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// UPDATE
	body = `{"name":"Updated Custom","systemMsg":"You are updated","userMsg":"Analyze v2: {{.Message}}","version":"v2"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/ai/prompts/custom-1", strings.NewReader(body))
	rr = httptest.NewRecorder()
	handler.handleAIPromptByID(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// DELETE
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/ai/prompts/custom-1", nil)
	rr = httptest.NewRecorder()
	handler.handleAIPromptByID(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAIAPIHealthEndpoint(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	aiQueue := newMemoryAIQueue()
	aiService := NewAIService(configStore, aiQueue, nil, nil, nil, nil)
	handler := NewAIAPIHandler(aiService, configStore, nil, &monitoring.Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/health", nil)
	rr := httptest.NewRecorder()
	handler.handleAIProviderHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAIAPIResultsEndpoint(t *testing.T) {
	configStore := newMemoryAIConfigStore()
	aiQueue := newMemoryAIQueue()
	aiService := NewAIService(configStore, aiQueue, nil, nil, nil, nil)
	handler := NewAIAPIHandler(aiService, configStore, nil, &monitoring.Config{})

	// Store a result
	_ = aiQueue.Enqueue("inc-api-test", "v1")
	_, _ = aiQueue.ClaimPending(1)
	_ = aiQueue.Complete("inc-api-test", monitoring.AIAnalysisResult{
		IncidentID: "inc-api-test",
		Analysis:   "Test analysis",
		CreatedAt:  time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/results/inc-api-test", nil)
	rr := httptest.NewRecorder()
	handler.handleAIResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp monitoring.APIResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if !resp.Success {
		t.Error("expected ok=true")
	}
}

// --- Incident Manager Integration Test ---

func TestIncidentManagerCallbackOnCreate(t *testing.T) {
	repo := monitoring.NewMemoryIncidentRepository()
	im := monitoring.NewIncidentManager(repo, nil)

	var callbackCalled bool
	var callbackIncidentID string

	im.SetOnIncidentCreated(func(incident monitoring.Incident) {
		callbackCalled = true
		callbackIncidentID = incident.ID
	})

	err := im.ProcessAlert("check-1", "Test Check", "api", "critical", "Server down", nil)
	if err != nil {
		t.Fatalf("ProcessAlert: %v", err)
	}

	if !callbackCalled {
		t.Error("expected callback to be called on incident creation")
	}
	if callbackIncidentID == "" {
		t.Error("expected callback to receive incident ID")
	}
}
