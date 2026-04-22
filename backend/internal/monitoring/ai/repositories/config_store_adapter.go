package repositories

import (
	"context"
	"sync"
	"time"

	"medics-health-check/backend/internal/monitoring/ai"
)

// AIConfigStoreInterface is the interface that both file-based and MongoDB config stores must implement.
// This is defined in the ai package, but we reference it here to ensure type compatibility.
type AIConfigStoreInterface interface {
	Get() ai.AIServiceConfig
	Update(mutator func(*ai.AIServiceConfig) error) error
}

// MongoAIConfigStoreAdapter adapts MongoAIConfigRepository to AIConfigStoreInterface.
// This allows the MongoDB repository to work with the existing AI service layer.
type MongoAIConfigStoreAdapter struct {
	repo *MongoAIConfigRepository
	mu   sync.RWMutex
	// Cache the service config for prompt templates
	serviceConfig ai.AIServiceConfig
}

// NewMongoAIConfigStoreAdapter creates a new adapter for the MongoDB AI config repository.
func NewMongoAIConfigStoreAdapter(repo *MongoAIConfigRepository) *MongoAIConfigStoreAdapter {
	adapter := &MongoAIConfigStoreAdapter{
		repo: repo,
	}
	// Initialize with default prompts
	adapter.serviceConfig = ai.DefaultAIServiceConfig()
	return adapter
}

// Get returns the current AI service configuration (thread-safe).
// It loads providers from MongoDB and merges with cached prompts.
func (a *MongoAIConfigStoreAdapter) Get() ai.AIServiceConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ctx := context.Background()
	providers, err := a.repo.List(ctx)
	if err != nil {
		// On error, return cached config with empty providers
		return a.serviceConfig
	}

	// Convert MongoDB providers to AI provider configs
	aiProviders := make([]ai.AIProviderConfig, len(providers))
	for i, p := range providers {
		aiProviders[i] = *convertFromRepositoriesProvider(p)
	}

	// Build the service config
	cfg := a.serviceConfig
	cfg.Providers = aiProviders

	return cfg
}

// Update atomically updates the AI service configuration.
// It updates providers in MongoDB and caches prompt templates in memory.
func (a *MongoAIConfigStoreAdapter) Update(mutator func(*ai.AIServiceConfig) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get current state
	cfg := a.Get()

	// Apply mutator
	if err := mutator(&cfg); err != nil {
		return err
	}

	// Update providers in MongoDB
	ctx := context.Background()

	// Get existing providers to determine which to add/update/delete
	existingProviders, err := a.repo.List(ctx)
	if err != nil {
		return err
	}

	existingMap := make(map[string]*AIProvider)
	for _, p := range existingProviders {
		existingMap[p.ID] = p
	}

	// Track providers in the new config
	newMap := make(map[string]bool)
	for _, pc := range cfg.Providers {
		newMap[pc.ID] = true

		mongoProvider := convertToRepositoriesProvider(&pc)
		mongoProvider.ID = pc.ID

		if _, exists := existingMap[pc.ID]; exists {
			// Preserve original timestamps for update
			mongoProvider.CreatedAt = existingMap[pc.ID].CreatedAt

			// Update existing provider
			if err := a.repo.Update(ctx, mongoProvider); err != nil {
				return err
			}
		} else {
			// Create new provider
			if err := a.repo.Create(ctx, mongoProvider); err != nil {
				return err
			}
		}
	}

	// Delete providers that are no longer in the config
	for id, existingProvider := range existingMap {
		if !newMap[id] {
			if err := a.repo.Delete(ctx, existingProvider.ID); err != nil {
				return err
			}
		}
	}

	// Cache the prompts in memory
	a.serviceConfig = cfg

	return nil
}

// --- Conversion Functions ---

// convertFromRepositoriesProvider converts repositories.AIProvider to ai.AIProviderConfig.
func convertFromRepositoriesProvider(p *AIProvider) *ai.AIProviderConfig {
	if p == nil {
		return nil
	}

	return &ai.AIProviderConfig{
		ID:          p.ID,
		Provider:    ai.AIProviderType(p.Provider),
		Name:        p.Name,
		APIKey:      p.APIKey,
		BaseURL:     p.BaseURL,
		Model:       p.Model,
		MaxTokens:   p.MaxTokens,
		Temperature: p.Temperature,
		Enabled:     p.Enabled,
		IsDefault:   p.Default,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// convertToRepositoriesProvider converts ai.AIProviderConfig to repositories.AIProvider.
func convertToRepositoriesProvider(pc *ai.AIProviderConfig) *AIProvider {
	if pc == nil {
		return nil
	}

	now := time.Now().UTC()
	return &AIProvider{
		Name:        pc.Name,
		Provider:    AIProviderType(pc.Provider),
		BaseURL:     pc.BaseURL,
		APIKey:      pc.APIKey,
		Model:       pc.Model,
		MaxTokens:   pc.MaxTokens,
		Temperature: pc.Temperature,
		Enabled:     pc.Enabled,
		Default:     pc.IsDefault,
		CreatedAt:   pc.CreatedAt,
		UpdatedAt:   now, // Always update timestamp on modification
	}
}
