package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AIAPIHandler handles all AI-related API endpoints.
type AIAPIHandler struct {
	aiService   *AIService
	configStore *AIConfigStore
	auditLogger *AuditLogger
	cfg         *Config
}

// NewAIAPIHandler creates a new AI API handler.
func NewAIAPIHandler(
	aiService *AIService,
	configStore *AIConfigStore,
	auditLogger *AuditLogger,
	cfg *Config,
) *AIAPIHandler {
	return &AIAPIHandler{
		aiService:   aiService,
		configStore: configStore,
		auditLogger: auditLogger,
		cfg:         cfg,
	}
}

// RegisterRoutes registers AI-related API routes.
func (h *AIAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/ai/config", h.handleAIConfig)
	mux.HandleFunc("/api/v1/ai/providers", h.handleAIProviders)
	mux.HandleFunc("/api/v1/ai/providers/", h.handleAIProviderByID)
	mux.HandleFunc("/api/v1/ai/prompts", h.handleAIPrompts)
	mux.HandleFunc("/api/v1/ai/prompts/", h.handleAIPromptByID)
	mux.HandleFunc("/api/v1/ai/analyze/", h.handleAnalyzeIncident)
	mux.HandleFunc("/api/v1/ai/health", h.handleAIProviderHealth)
	mux.HandleFunc("/api/v1/ai/results/", h.handleAIResults)
}

// --- AI Config ---

// GET /api/v1/ai/config — get AI configuration (keys masked)
// PUT /api/v1/ai/config — update AI settings (enable/disable, concurrency, timeouts)
func (h *AIAPIHandler) handleAIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := h.configStore.Get()
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(cfg.toSafeView()))

	case http.MethodPut:
		if !isRequestAuthorized(h.cfg.Auth, r) {
			requestAuth(w)
			return
		}

		var update struct {
			Enabled         *bool   `json:"enabled"`
			AutoAnalyze     *bool   `json:"autoAnalyze"`
			MaxConcurrent   *int    `json:"maxConcurrent"`
			TimeoutSeconds  *int    `json:"timeoutSeconds"`
			RetryCount      *int    `json:"retryCount"`
			RetryDelayMs    *int    `json:"retryDelayMs"`
			DefaultPromptID *string `json:"defaultPromptId"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&update); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			if update.Enabled != nil {
				cfg.Enabled = *update.Enabled
			}
			if update.AutoAnalyze != nil {
				cfg.AutoAnalyze = *update.AutoAnalyze
			}
			if update.MaxConcurrent != nil {
				cfg.MaxConcurrent = *update.MaxConcurrent
			}
			if update.TimeoutSeconds != nil {
				cfg.TimeoutSeconds = *update.TimeoutSeconds
			}
			if update.RetryCount != nil {
				cfg.RetryCount = *update.RetryCount
			}
			if update.RetryDelayMs != nil {
				cfg.RetryDelayMs = *update.RetryDelayMs
			}
			if update.DefaultPromptID != nil {
				cfg.DefaultPromptID = *update.DefaultPromptID
			}
			return cfg.validate()
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		// Reload providers if settings changed
		if h.aiService != nil {
			h.aiService.ReloadProviders()
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_config.updated", actor, "ai_config", "", nil)
		}

		cfg := h.configStore.Get()
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(cfg.toSafeView()))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- Providers CRUD ---

// GET /api/v1/ai/providers — list all providers (keys masked)
// POST /api/v1/ai/providers — add a new provider
func (h *AIAPIHandler) handleAIProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := h.configStore.Get()
		safe := cfg.toSafeView()
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(safe.Providers))

	case http.MethodPost:
		if !isRequestAuthorized(h.cfg.Auth, r) {
			requestAuth(w)
			return
		}

		var provider AIProviderConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&provider); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		now := time.Now().UTC()
		provider.CreatedAt = now
		provider.UpdatedAt = now

		if err := provider.validate(); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			// Check for duplicate ID
			for _, p := range cfg.Providers {
				if p.ID == provider.ID {
					return fmt.Errorf("provider with ID %q already exists", provider.ID)
				}
			}

			// If setting as default, unset others
			if provider.IsDefault {
				for i := range cfg.Providers {
					cfg.Providers[i].IsDefault = false
				}
			}

			cfg.Providers = append(cfg.Providers, provider)
			return nil
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		if h.aiService != nil {
			h.aiService.ReloadProviders()
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_provider.created", actor, "ai_provider", provider.ID, map[string]interface{}{
				"provider": string(provider.Provider),
				"model":    provider.Model,
			})
		}

		// Return safe view (masked key)
		safeProvider := SafeAIProviderView{
			ID:          provider.ID,
			Provider:    provider.Provider,
			Name:        provider.Name,
			APIKey:      maskAPIKey(provider.APIKey),
			BaseURL:     provider.BaseURL,
			Model:       provider.Model,
			MaxTokens:   provider.MaxTokens,
			Temperature: provider.Temperature,
			Enabled:     provider.Enabled,
			IsDefault:   provider.IsDefault,
			CreatedAt:   provider.CreatedAt,
			UpdatedAt:   provider.UpdatedAt,
		}

		writeAPIResponse(w, http.StatusCreated, NewAPIResponse(safeProvider))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// PUT /api/v1/ai/providers/{id} — update a provider
// DELETE /api/v1/ai/providers/{id} — remove a provider
func (h *AIAPIHandler) handleAIProviderByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/providers/")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing provider ID"))
		return
	}

	if !isRequestAuthorized(h.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var provider AIProviderConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&provider); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		provider.ID = id
		provider.UpdatedAt = time.Now().UTC()

		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			found := false
			for i := range cfg.Providers {
				if cfg.Providers[i].ID == id {
					// Preserve creation timestamp
					provider.CreatedAt = cfg.Providers[i].CreatedAt

					// If API key is empty or masked, keep the existing key
					if provider.APIKey == "" || strings.Contains(provider.APIKey, "****") {
						provider.APIKey = cfg.Providers[i].APIKey
					}

					// Validate after key preservation
					if err := provider.validate(); err != nil {
						return err
					}

					// If setting as default, unset others
					if provider.IsDefault {
						for j := range cfg.Providers {
							if j != i {
								cfg.Providers[j].IsDefault = false
							}
						}
					}

					cfg.Providers[i] = provider
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("provider %q not found", id)
			}
			return nil
		})
		if err != nil {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}

		if h.aiService != nil {
			h.aiService.ReloadProviders()
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_provider.updated", actor, "ai_provider", id, map[string]interface{}{
				"model": provider.Model,
			})
		}

		safeProvider := SafeAIProviderView{
			ID:          provider.ID,
			Provider:    provider.Provider,
			Name:        provider.Name,
			APIKey:      maskAPIKey(provider.APIKey),
			BaseURL:     provider.BaseURL,
			Model:       provider.Model,
			MaxTokens:   provider.MaxTokens,
			Temperature: provider.Temperature,
			Enabled:     provider.Enabled,
			IsDefault:   provider.IsDefault,
			CreatedAt:   provider.CreatedAt,
			UpdatedAt:   provider.UpdatedAt,
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(safeProvider))

	case http.MethodDelete:
		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			for i := range cfg.Providers {
				if cfg.Providers[i].ID == id {
					cfg.Providers = append(cfg.Providers[:i], cfg.Providers[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf("provider %q not found", id)
		})
		if err != nil {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}

		if h.aiService != nil {
			h.aiService.ReloadProviders()
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_provider.deleted", actor, "ai_provider", id, nil)
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"deleted": id}))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- Prompt Templates CRUD ---

// GET /api/v1/ai/prompts — list all prompt templates
// POST /api/v1/ai/prompts — add a new prompt template
func (h *AIAPIHandler) handleAIPrompts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := h.configStore.Get()
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(cfg.Prompts))

	case http.MethodPost:
		if !isRequestAuthorized(h.cfg.Auth, r) {
			requestAuth(w)
			return
		}

		var prompt AIPromptTemplate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&prompt); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		if prompt.ID == "" || prompt.Name == "" || prompt.UserMsg == "" {
			writeAPIError(w, http.StatusBadRequest, fmt.Errorf("id, name, and userMsg are required"))
			return
		}

		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			for _, p := range cfg.Prompts {
				if p.ID == prompt.ID {
					return fmt.Errorf("prompt with ID %q already exists", prompt.ID)
				}
			}

			if prompt.IsDefault {
				for i := range cfg.Prompts {
					cfg.Prompts[i].IsDefault = false
				}
			}

			cfg.Prompts = append(cfg.Prompts, prompt)
			return nil
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_prompt.created", actor, "ai_prompt", prompt.ID, nil)
		}

		writeAPIResponse(w, http.StatusCreated, NewAPIResponse(prompt))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// PUT /api/v1/ai/prompts/{id} — update a prompt template
// DELETE /api/v1/ai/prompts/{id} — remove a prompt template
func (h *AIAPIHandler) handleAIPromptByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/prompts/")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing prompt ID"))
		return
	}

	if !isRequestAuthorized(h.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var prompt AIPromptTemplate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&prompt); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		prompt.ID = id

		if prompt.Name == "" || prompt.UserMsg == "" {
			writeAPIError(w, http.StatusBadRequest, fmt.Errorf("name and userMsg are required"))
			return
		}

		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			found := false
			for i := range cfg.Prompts {
				if cfg.Prompts[i].ID == id {
					if prompt.IsDefault {
						for j := range cfg.Prompts {
							if j != i {
								cfg.Prompts[j].IsDefault = false
							}
						}
					}
					cfg.Prompts[i] = prompt
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("prompt %q not found", id)
			}
			return nil
		})
		if err != nil {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_prompt.updated", actor, "ai_prompt", id, nil)
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(prompt))

	case http.MethodDelete:
		err := h.configStore.Update(func(cfg *AIServiceConfig) error {
			for i := range cfg.Prompts {
				if cfg.Prompts[i].ID == id {
					cfg.Prompts = append(cfg.Prompts[:i], cfg.Prompts[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf("prompt %q not found", id)
		})
		if err != nil {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}

		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai_prompt.deleted", actor, "ai_prompt", id, nil)
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"deleted": id}))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- On-Demand Analysis ---

// POST /api/v1/ai/analyze/{incidentId} — trigger AI analysis for a specific incident
func (h *AIAPIHandler) handleAnalyzeIncident(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !isRequestAuthorized(h.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	incidentID := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/analyze/")
	if incidentID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing incident ID"))
		return
	}

	var opts struct {
		ProviderID string `json:"providerId,omitempty"` // optional: use specific provider
		PromptID   string `json:"promptId,omitempty"`   // optional: use specific prompt
	}
	// Body is optional
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&opts)

	if h.aiService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("AI service not configured"))
		return
	}

	result, err := h.aiService.AnalyzeIncident(r.Context(), incidentID, opts.ProviderID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	if h.auditLogger != nil {
		actor := ExtractActorFromRequest(r, h.cfg)
		_ = h.auditLogger.Log("ai.analysis.triggered", actor, "incident", incidentID, map[string]interface{}{
			"providerId": opts.ProviderID,
		})
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(result))
}

// --- Provider Health ---

// GET /api/v1/ai/health — check provider connectivity
func (h *AIAPIHandler) handleAIProviderHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if h.aiService == nil {
		writeAPIResponse(w, http.StatusOK, NewAPIResponse([]ProviderHealth{}))
		return
	}

	results := h.aiService.CheckProviderHealth(r.Context())
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(results))
}

// --- AI Analysis Results ---

// GET /api/v1/ai/results/{incidentId} — get analysis results for an incident
func (h *AIAPIHandler) handleAIResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	incidentID := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/results/")
	if incidentID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing incident ID"))
		return
	}

	cfg := h.configStore.Get()
	_ = cfg // referenced for consistency

	// Get results from the AI queue's results store
	if h.aiService == nil || h.aiService.aiQueue == nil {
		writeAPIResponse(w, http.StatusOK, NewAPIResponse([]AIAnalysisResult{}))
		return
	}

	results := h.aiService.aiQueue.GetResults(incidentID)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(results))
}
