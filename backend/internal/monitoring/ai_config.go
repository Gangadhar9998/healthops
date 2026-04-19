package monitoring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --- BYOK AI Provider Configuration ---

// AIProviderType identifies supported AI providers.
type AIProviderType string

const (
	AIProviderOpenAI    AIProviderType = "openai"
	AIProviderAnthropic AIProviderType = "anthropic"
	AIProviderGoogle    AIProviderType = "google"
	AIProviderOllama    AIProviderType = "ollama"
	AIProviderCustom    AIProviderType = "custom"
)

// AIProviderConfig holds BYOK configuration for an AI provider.
type AIProviderConfig struct {
	ID          string         `json:"id"`
	Provider    AIProviderType `json:"provider"`
	Name        string         `json:"name"`
	APIKey      string         `json:"apiKey,omitempty"`      // encrypted at rest, masked in API responses
	BaseURL     string         `json:"baseURL,omitempty"`     // custom endpoint (required for ollama/custom)
	Model       string         `json:"model"`                 // e.g. gpt-4o, claude-sonnet-4-20250514, gemini-2.0-flash
	MaxTokens   int            `json:"maxTokens,omitempty"`   // response token limit
	Temperature float64        `json:"temperature,omitempty"` // 0.0–2.0
	Enabled     bool           `json:"enabled"`
	IsDefault   bool           `json:"isDefault,omitempty"` // primary provider for analysis
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// AIPromptTemplate holds configurable prompt templates for different analysis types.
type AIPromptTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SystemMsg   string `json:"systemMsg"`
	UserMsg     string `json:"userMsg"` // Go template with {{.IncidentID}}, {{.CheckName}}, {{.Evidence}}, etc.
	Version     string `json:"version"`
	IsDefault   bool   `json:"isDefault,omitempty"`
}

// AIServiceConfig aggregates all AI layer settings.
type AIServiceConfig struct {
	Enabled         bool               `json:"enabled"`
	AutoAnalyze     bool               `json:"autoAnalyze"`    // automatically analyze new incidents
	MaxConcurrent   int                `json:"maxConcurrent"`  // max parallel AI calls
	TimeoutSeconds  int                `json:"timeoutSeconds"` // per-request timeout
	RetryCount      int                `json:"retryCount"`     // retries on transient failures
	RetryDelayMs    int                `json:"retryDelayMs"`   // delay between retries
	Providers       []AIProviderConfig `json:"providers"`
	DefaultPromptID string             `json:"defaultPromptId,omitempty"`
	Prompts         []AIPromptTemplate `json:"prompts"`
}

// SafeAIConfigView is the API-safe version (keys masked).
type SafeAIConfigView struct {
	Enabled         bool                 `json:"enabled"`
	AutoAnalyze     bool                 `json:"autoAnalyze"`
	MaxConcurrent   int                  `json:"maxConcurrent"`
	TimeoutSeconds  int                  `json:"timeoutSeconds"`
	RetryCount      int                  `json:"retryCount"`
	RetryDelayMs    int                  `json:"retryDelayMs"`
	Providers       []SafeAIProviderView `json:"providers"`
	DefaultPromptID string               `json:"defaultPromptId,omitempty"`
	Prompts         []AIPromptTemplate   `json:"prompts"`
}

// SafeAIProviderView masks API keys for safe display.
type SafeAIProviderView struct {
	ID          string         `json:"id"`
	Provider    AIProviderType `json:"provider"`
	Name        string         `json:"name"`
	APIKey      string         `json:"apiKey"` // masked: "sk-...****abcd"
	BaseURL     string         `json:"baseURL,omitempty"`
	Model       string         `json:"model"`
	MaxTokens   int            `json:"maxTokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Enabled     bool           `json:"enabled"`
	IsDefault   bool           `json:"isDefault,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// --- Defaults ---

func DefaultAIServiceConfig() AIServiceConfig {
	return AIServiceConfig{
		Enabled:        false,
		AutoAnalyze:    true,
		MaxConcurrent:  2,
		TimeoutSeconds: 30,
		RetryCount:     2,
		RetryDelayMs:   1000,
		Providers:      []AIProviderConfig{},
		Prompts:        defaultPromptTemplates(),
	}
}

func defaultPromptTemplates() []AIPromptTemplate {
	return []AIPromptTemplate{
		{
			ID:          "incident-analysis-v1",
			Name:        "Incident Root Cause Analysis",
			Description: "Analyzes incident evidence to determine root cause and provide actionable suggestions",
			SystemMsg: `You are an expert Site Reliability Engineer (SRE) and database administrator. 
Your job is to analyze monitoring incidents by examining the evidence provided (metrics, logs, process lists, etc.) and determine:
1. The root cause of the issue
2. The severity and potential impact
3. Specific, actionable remediation steps
4. Whether this is likely a symptom of a larger issue

Be concise, technical, and actionable. Prioritize the most likely root cause. 
If evidence is insufficient, state what additional data would help.
Format your response as structured JSON with fields: rootCause, impact, severity, suggestions (array), additionalDataNeeded (array), confidence (high/medium/low).`,
			UserMsg: `Analyze this monitoring incident:

**Incident ID**: {{.IncidentID}}
**Check Name**: {{.CheckName}}
**Check Type**: {{.CheckType}}
**Severity**: {{.Severity}}
**Message**: {{.Message}}
**Started At**: {{.StartedAt}}
**Duration**: {{.Duration}}

**Evidence Snapshots**:
{{.Evidence}}

**Recent Check Results**:
{{.RecentResults}}

Provide your analysis as structured JSON.`,
			Version:   "v1",
			IsDefault: true,
		},
		{
			ID:          "mysql-analysis-v1",
			Name:        "MySQL Performance Analysis",
			Description: "Specialized analysis for MySQL monitoring incidents with metrics deep-dive",
			SystemMsg: `You are an expert MySQL DBA and performance engineer.
Analyze the MySQL metrics, deltas, process list, and statement analysis to identify:
1. Root cause of the performance issue or alert
2. Whether this is a transient spike or sustained degradation
3. Specific MySQL tuning recommendations (variables, indexes, queries)
4. Connection pool and thread management advice
5. Capacity planning implications

Reference specific metric values and thresholds in your analysis.
Format your response as structured JSON with fields: rootCause, mysqlSpecific (object with connectionAnalysis, queryAnalysis, lockAnalysis, capacityAnalysis), suggestions (array), urgency (immediate/soon/planned), confidence (high/medium/low).`,
			UserMsg: `Analyze this MySQL monitoring incident:

**Incident ID**: {{.IncidentID}}
**Check Name**: {{.CheckName}}
**Alert Rule**: {{.RuleCode}}
**Severity**: {{.Severity}}
**Message**: {{.Message}}

**Latest MySQL Sample**:
{{.LatestSample}}

**Recent Deltas (rate of change)**:
{{.RecentDeltas}}

**Process List** (active queries):
{{.ProcessList}}

**Statement Analysis** (top queries):
{{.StatementAnalysis}}

Provide your analysis as structured JSON.`,
			Version:   "v1",
			IsDefault: false,
		},
	}
}

// --- Validation ---

func (c *AIProviderConfig) validate() error {
	if c.ID == "" {
		return fmt.Errorf("provider ID is required")
	}
	if c.Name == "" {
		return fmt.Errorf("provider name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}

	switch c.Provider {
	case AIProviderOpenAI, AIProviderAnthropic, AIProviderGoogle:
		if c.APIKey == "" {
			return fmt.Errorf("API key is required for %s provider", c.Provider)
		}
	case AIProviderOllama:
		if c.BaseURL == "" {
			return fmt.Errorf("base URL is required for Ollama provider")
		}
	case AIProviderCustom:
		if c.BaseURL == "" {
			return fmt.Errorf("base URL is required for custom provider")
		}
	default:
		return fmt.Errorf("unsupported provider: %s (supported: openai, anthropic, google, ollama, custom)", c.Provider)
	}

	if c.Temperature < 0 || c.Temperature > 2.0 {
		return fmt.Errorf("temperature must be between 0.0 and 2.0")
	}
	if c.MaxTokens < 0 || c.MaxTokens > 128000 {
		return fmt.Errorf("maxTokens must be between 0 and 128000")
	}

	return nil
}

func (c *AIServiceConfig) validate() error {
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 20 {
		return fmt.Errorf("maxConcurrent must be between 1 and 20")
	}
	if c.TimeoutSeconds < 5 || c.TimeoutSeconds > 300 {
		return fmt.Errorf("timeoutSeconds must be between 5 and 300")
	}
	if c.RetryCount < 0 || c.RetryCount > 10 {
		return fmt.Errorf("retryCount must be between 0 and 10")
	}
	if c.RetryDelayMs < 100 || c.RetryDelayMs > 30000 {
		return fmt.Errorf("retryDelayMs must be between 100 and 30000")
	}

	defaultCount := 0
	for _, p := range c.Providers {
		if err := p.validate(); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
		if p.IsDefault {
			defaultCount++
		}
	}
	if defaultCount > 1 {
		return fmt.Errorf("only one provider can be the default")
	}

	return nil
}

// --- Masking ---

func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + strings.Repeat("*", 4) + key[len(key)-4:]
}

func (c *AIServiceConfig) toSafeView() SafeAIConfigView {
	providers := make([]SafeAIProviderView, len(c.Providers))
	for i, p := range c.Providers {
		providers[i] = SafeAIProviderView{
			ID:          p.ID,
			Provider:    p.Provider,
			Name:        p.Name,
			APIKey:      maskAPIKey(p.APIKey),
			BaseURL:     p.BaseURL,
			Model:       p.Model,
			MaxTokens:   p.MaxTokens,
			Temperature: p.Temperature,
			Enabled:     p.Enabled,
			IsDefault:   p.IsDefault,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		}
	}

	promptsCopy := make([]AIPromptTemplate, len(c.Prompts))
	copy(promptsCopy, c.Prompts)

	return SafeAIConfigView{
		Enabled:         c.Enabled,
		AutoAnalyze:     c.AutoAnalyze,
		MaxConcurrent:   c.MaxConcurrent,
		TimeoutSeconds:  c.TimeoutSeconds,
		RetryCount:      c.RetryCount,
		RetryDelayMs:    c.RetryDelayMs,
		Providers:       providers,
		DefaultPromptID: c.DefaultPromptID,
		Prompts:         promptsCopy,
	}
}

// --- Persistence ---

// AIConfigStore persists AI configuration to disk with encryption for API keys.
type AIConfigStore struct {
	mu         sync.RWMutex
	configPath string
	config     AIServiceConfig
	encKey     []byte // 32-byte AES key for encrypting API keys at rest
}

// NewAIConfigStore loads or creates AI config.
func NewAIConfigStore(dataDir string) (*AIConfigStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ai config dir: %w", err)
	}

	configPath := filepath.Join(dataDir, "ai_config.json")
	keyPath := filepath.Join(dataDir, ".ai_enc_key")

	encKey, err := loadOrCreateEncKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("init encryption key: %w", err)
	}

	store := &AIConfigStore{
		configPath: configPath,
		encKey:     encKey,
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func loadOrCreateEncKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}

	encoded := hex.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("save encryption key: %w", err)
	}

	return key, nil
}

func (s *AIConfigStore) load() error {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.config = DefaultAIServiceConfig()
			return nil
		}
		return fmt.Errorf("read ai config: %w", err)
	}

	var cfg AIServiceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse ai config: %w", err)
	}

	// Decrypt API keys
	for i := range cfg.Providers {
		if cfg.Providers[i].APIKey != "" {
			decrypted, err := decryptString(s.encKey, cfg.Providers[i].APIKey)
			if err != nil {
				// If decryption fails, the key might be stored in plaintext (first migration)
				continue
			}
			cfg.Providers[i].APIKey = decrypted
		}
	}

	s.config = cfg
	return nil
}

func (s *AIConfigStore) save() error {
	// Clone config for writing — encrypt API keys
	cfg := s.config
	providers := make([]AIProviderConfig, len(cfg.Providers))
	copy(providers, cfg.Providers)

	for i := range providers {
		if providers[i].APIKey != "" {
			encrypted, err := encryptString(s.encKey, providers[i].APIKey)
			if err != nil {
				return fmt.Errorf("encrypt api key for %s: %w", providers[i].ID, err)
			}
			providers[i].APIKey = encrypted
		}
	}
	cfg.Providers = providers

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ai config: %w", err)
	}

	tmpPath := s.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write ai config tmp: %w", err)
	}
	if err := os.Rename(tmpPath, s.configPath); err != nil {
		return fmt.Errorf("rename ai config: %w", err)
	}

	return nil
}

// Get returns the current config (thread-safe).
func (s *AIConfigStore) Get() AIServiceConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.config
	providers := make([]AIProviderConfig, len(cfg.Providers))
	copy(providers, cfg.Providers)
	cfg.Providers = providers

	prompts := make([]AIPromptTemplate, len(cfg.Prompts))
	copy(prompts, cfg.Prompts)
	cfg.Prompts = prompts

	return cfg
}

// Update atomically updates the AI config.
func (s *AIConfigStore) Update(mutator func(*AIServiceConfig) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfgCopy := s.config
	providers := make([]AIProviderConfig, len(cfgCopy.Providers))
	copy(providers, cfgCopy.Providers)
	cfgCopy.Providers = providers

	prompts := make([]AIPromptTemplate, len(cfgCopy.Prompts))
	copy(prompts, cfgCopy.Prompts)
	cfgCopy.Prompts = prompts

	if err := mutator(&cfgCopy); err != nil {
		return err
	}

	s.config = cfgCopy
	return s.save()
}

// --- AES-256-GCM Encryption ---

func encryptString(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

func decryptString(key []byte, cipherHex string) (string, error) {
	ciphertext, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
