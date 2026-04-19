package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// --- AI Provider Interface ---

// AIProvider abstracts AI completion calls across providers.
type AIProvider interface {
	// Name returns the provider identifier.
	Name() string
	// Analyze sends an analysis request and returns the raw response text.
	Analyze(ctx context.Context, systemMsg, userMsg string) (string, error)
	// IsHealthy checks if the provider is reachable (non-blocking, best-effort).
	IsHealthy(ctx context.Context) bool
}

// --- OpenAI-Compatible Provider ---
// Works with: OpenAI, Azure OpenAI, any OpenAI-compatible API (vLLM, LiteLLM, etc.)

type openAIProvider struct {
	cfg    AIProviderConfig
	client *http.Client
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIChatMsg `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature"`
}

type openAIChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func newOpenAIProvider(cfg AIProviderConfig, timeout time.Duration) *openAIProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	return &openAIProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *openAIProvider) Name() string {
	return fmt.Sprintf("openai:%s", p.cfg.Model)
}

func (p *openAIProvider) Analyze(ctx context.Context, systemMsg, userMsg string) (string, error) {
	reqBody := openAIChatRequest{
		Model: p.cfg.Model,
		Messages: []openAIChatMsg{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Temperature: p.cfg.Temperature,
	}
	if p.cfg.MaxTokens > 0 {
		reqBody.MaxTokens = p.cfg.MaxTokens
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := p.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s (%s)", chatResp.Error.Message, chatResp.Error.Type)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (p *openAIProvider) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// --- Anthropic Provider ---

type anthropicProvider struct {
	cfg    AIProviderConfig
	client *http.Client
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func newAnthropicProvider(cfg AIProviderConfig, timeout time.Duration) *anthropicProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}

	return &anthropicProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *anthropicProvider) Name() string {
	return fmt.Sprintf("anthropic:%s", p.cfg.Model)
}

func (p *anthropicProvider) Analyze(ctx context.Context, systemMsg, userMsg string) (string, error) {
	reqBody := anthropicRequest{
		Model:     p.cfg.Model,
		MaxTokens: p.cfg.MaxTokens,
		System:    systemMsg,
		Messages: []anthropicMessage{
			{Role: "user", Content: userMsg},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := p.cfg.BaseURL + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if anthropicResp.Error != nil {
		return "", fmt.Errorf("API error: %s (%s)", anthropicResp.Error.Message, anthropicResp.Error.Type)
	}

	var parts []string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	return strings.Join(parts, "\n"), nil
}

func (p *anthropicProvider) IsHealthy(ctx context.Context) bool {
	// Anthropic doesn't have a simple health endpoint; use a minimal request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/messages", nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// A 400 (bad request) means the API is up; 401/403 means bad key; 5xx means down
	return resp.StatusCode < 500
}

// --- Google Gemini Provider ---

type googleProvider struct {
	cfg    AIProviderConfig
	client *http.Client
}

type googleRequest struct {
	Contents          []googleContent      `json:"contents"`
	SystemInstruction *googleContent       `json:"systemInstruction,omitempty"`
	GenerationConfig  *googleGenerationCfg `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenerationCfg struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func newGoogleProvider(cfg AIProviderConfig, timeout time.Duration) *googleProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	return &googleProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *googleProvider) Name() string {
	return fmt.Sprintf("google:%s", p.cfg.Model)
}

func (p *googleProvider) Analyze(ctx context.Context, systemMsg, userMsg string) (string, error) {
	reqBody := googleRequest{
		Contents: []googleContent{
			{
				Role:  "user",
				Parts: []googlePart{{Text: userMsg}},
			},
		},
	}

	if systemMsg != "" {
		reqBody.SystemInstruction = &googleContent{
			Parts: []googlePart{{Text: systemMsg}},
		}
	}

	if p.cfg.Temperature > 0 || p.cfg.MaxTokens > 0 {
		reqBody.GenerationConfig = &googleGenerationCfg{
			Temperature:     p.cfg.Temperature,
			MaxOutputTokens: p.cfg.MaxTokens,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.cfg.BaseURL, p.cfg.Model, p.cfg.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var googleResp googleResponse
	if err := json.Unmarshal(respBody, &googleResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if googleResp.Error != nil {
		return "", fmt.Errorf("API error: %s (code %d)", googleResp.Error.Message, googleResp.Error.Code)
	}

	if len(googleResp.Candidates) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	var parts []string
	for _, part := range googleResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("empty text in response")
	}

	return strings.Join(parts, "\n"), nil
}

func (p *googleProvider) IsHealthy(ctx context.Context) bool {
	url := fmt.Sprintf("%s/models?key=%s", p.cfg.BaseURL, p.cfg.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// --- Ollama Provider (local, no API key needed) ---

type ollamaProvider struct {
	cfg    AIProviderConfig
	client *http.Client
}

type ollamaRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	System  string         `json:"system,omitempty"`
	Stream  bool           `json:"stream"`
	Options *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

func newOllamaProvider(cfg AIProviderConfig, timeout time.Duration) *ollamaProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	cfg.BaseURL = strings.TrimRight(baseURL, "/")

	return &ollamaProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *ollamaProvider) Name() string {
	return fmt.Sprintf("ollama:%s", p.cfg.Model)
}

func (p *ollamaProvider) Analyze(ctx context.Context, systemMsg, userMsg string) (string, error) {
	reqBody := ollamaRequest{
		Model:  p.cfg.Model,
		Prompt: userMsg,
		System: systemMsg,
		Stream: false,
	}

	if p.cfg.Temperature > 0 || p.cfg.MaxTokens > 0 {
		reqBody.Options = &ollamaOptions{
			Temperature: p.cfg.Temperature,
			NumPredict:  p.cfg.MaxTokens,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := p.cfg.BaseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", ollamaResp.Error)
	}

	return ollamaResp.Response, nil
}

func (p *ollamaProvider) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// --- Provider Factory ---

// NewAIProvider creates the correct provider implementation from config.
func NewAIProvider(cfg AIProviderConfig, timeout time.Duration) (AIProvider, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	switch cfg.Provider {
	case AIProviderOpenAI:
		return newOpenAIProvider(cfg, timeout), nil
	case AIProviderAnthropic:
		return newAnthropicProvider(cfg, timeout), nil
	case AIProviderGoogle:
		return newGoogleProvider(cfg, timeout), nil
	case AIProviderOllama:
		return newOllamaProvider(cfg, timeout), nil
	case AIProviderCustom:
		// Custom providers use OpenAI-compatible API format
		return newOpenAIProvider(cfg, timeout), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Provider)
	}
}

// --- Helpers ---

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
