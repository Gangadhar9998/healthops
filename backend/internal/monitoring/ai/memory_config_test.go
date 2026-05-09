package ai

import "sync"

type memoryAIConfigStore struct {
	mu     sync.RWMutex
	config AIServiceConfig
}

func newMemoryAIConfigStore() *memoryAIConfigStore {
	return &memoryAIConfigStore{config: DefaultAIServiceConfig()}
}

func (s *memoryAIConfigStore) Get() AIServiceConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAIServiceConfig(s.config)
}

func (s *memoryAIConfigStore) Update(mutator func(*AIServiceConfig) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneAIServiceConfig(s.config)
	if err := mutator(&next); err != nil {
		return err
	}
	s.config = next
	return nil
}

func cloneAIServiceConfig(cfg AIServiceConfig) AIServiceConfig {
	providers := make([]AIProviderConfig, len(cfg.Providers))
	copy(providers, cfg.Providers)
	cfg.Providers = providers

	prompts := make([]AIPromptTemplate, len(cfg.Prompts))
	copy(prompts, cfg.Prompts)
	cfg.Prompts = prompts
	return cfg
}
