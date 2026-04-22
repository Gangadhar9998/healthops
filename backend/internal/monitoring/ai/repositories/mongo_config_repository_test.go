package repositories

import (
	"context"
	"os"
	"testing"
)

// TestMongoAIConfigRepository tests the MongoDB AI config repository.
func TestMongoAIConfigRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Get MongoDB URI from environment or use default
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	// Create temporary directory for test encryption key
	tempDir := t.TempDir()

	// Create repository
	cfg := MongoAIConfigRepositoryConfig{
		MongoURI:       mongoURI,
		DatabaseName:   "healthops_test",
		CollectionName: "test_ai_config",
		DataDir:        tempDir,
		RetentionDays:  7,
	}

	repo, err := NewMongoAIConfigRepository(cfg)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer func() {
		// Clean up test collection
		ctx := context.Background()
		repo.collection.Database().Drop(ctx)
		repo.Close()
	}()

	ctx := context.Background()

	// Test Create
	t.Run("Create provider", func(t *testing.T) {
		provider := &AIProvider{
			ID:          "test-openai-1",
			Name:        "Test OpenAI",
			Provider:    AIProviderOpenAI,
			APIKey:      "sk-test-key-1234567890abcdef",
			Model:       "gpt-4o",
			MaxTokens:   4096,
			Temperature: 0.7,
			Enabled:     true,
			Default:     true,
			Metadata: map[string]interface{}{
				"region": "us-east-1",
			},
		}

		err := repo.Create(ctx, provider)
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}

		// Verify the provider was created
		retrieved, err := repo.Get(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve provider: %v", err)
		}

		if retrieved.Name != provider.Name {
			t.Errorf("Expected name %s, got %s", provider.Name, retrieved.Name)
		}
		if retrieved.APIKey != provider.APIKey {
			t.Errorf("API key was not properly decrypted")
		}
		if !retrieved.Default {
			t.Errorf("Expected provider to be marked as default")
		}
	})

	// Test GetDefault
	t.Run("Get default provider", func(t *testing.T) {
		defaultProvider, err := repo.GetDefault(ctx)
		if err != nil {
			t.Fatalf("Failed to get default provider: %v", err)
		}

		if defaultProvider.ID != "test-openai-1" {
			t.Errorf("Expected default provider ID test-openai-1, got %s", defaultProvider.ID)
		}
		if !defaultProvider.Default {
			t.Errorf("Expected provider to be marked as default")
		}
	})

	// Test Create second provider (should not be default)
	t.Run("Create second provider", func(t *testing.T) {
		provider := &AIProvider{
			ID:          "test-anthropic-1",
			Name:        "Test Anthropic",
			Provider:    AIProviderAnthropic,
			APIKey:      "sk-ant-test-key-1234567890abcdef",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   8192,
			Temperature: 0.5,
			Enabled:     true,
			Default:     false,
		}

		err := repo.Create(ctx, provider)
		if err != nil {
			t.Fatalf("Failed to create second provider: %v", err)
		}

		// Verify only one default exists
		defaultProvider, err := repo.GetDefault(ctx)
		if err != nil {
			t.Fatalf("Failed to get default provider: %v", err)
		}

		if defaultProvider.ID != "test-openai-1" {
			t.Errorf("Default should still be test-openai-1, got %s", defaultProvider.ID)
		}
	})

	// Test List
	t.Run("List all providers", func(t *testing.T) {
		providers, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("Failed to list providers: %v", err)
		}

		if len(providers) != 2 {
			t.Errorf("Expected 2 providers, got %d", len(providers))
		}

		// Verify all API keys are decrypted
		for _, p := range providers {
			if p.APIKey == "" {
				t.Errorf("Provider %s has empty API key after decryption", p.ID)
			}
		}
	})

	// Test ListEnabled
	t.Run("List enabled providers", func(t *testing.T) {
		// Disable one provider
		provider, err := repo.Get(ctx, "test-anthropic-1")
		if err != nil {
			t.Fatalf("Failed to get provider: %v", err)
		}
		provider.Enabled = false
		err = repo.Update(ctx, provider)
		if err != nil {
			t.Fatalf("Failed to update provider: %v", err)
		}

		enabled, err := repo.ListEnabled(ctx)
		if err != nil {
			t.Fatalf("Failed to list enabled providers: %v", err)
		}

		if len(enabled) != 1 {
			t.Errorf("Expected 1 enabled provider, got %d", len(enabled))
		}
	})

	// Test SetDefault
	t.Run("Set new default provider", func(t *testing.T) {
		err := repo.SetDefault(ctx, "test-anthropic-1")
		if err != nil {
			t.Fatalf("Failed to set default: %v", err)
		}

		// Verify the old default is unmarked
		oldDefault, err := repo.Get(ctx, "test-openai-1")
		if err != nil {
			t.Fatalf("Failed to get old default: %v", err)
		}
		if oldDefault.Default {
			t.Errorf("Old default should be unmarked")
		}

		// Verify the new default is marked
		newDefault, err := repo.GetDefault(ctx)
		if err != nil {
			t.Fatalf("Failed to get new default: %v", err)
		}
		if newDefault.ID != "test-anthropic-1" {
			t.Errorf("Expected new default to be test-anthropic-1, got %s", newDefault.ID)
		}
	})

	// Test Update
	t.Run("Update provider", func(t *testing.T) {
		provider, err := repo.Get(ctx, "test-openai-1")
		if err != nil {
			t.Fatalf("Failed to get provider: %v", err)
		}

		originalName := provider.Name
		provider.Name = "Updated OpenAI"
		provider.Temperature = 1.0

		err = repo.Update(ctx, provider)
		if err != nil {
			t.Fatalf("Failed to update provider: %v", err)
		}

		updated, err := repo.Get(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated provider: %v", err)
		}

		if updated.Name == originalName {
			t.Errorf("Provider name was not updated")
		}
		if updated.Temperature != 1.0 {
			t.Errorf("Temperature was not updated")
		}

		// Verify API key is still decrypted correctly
		if updated.APIKey == "" {
			t.Errorf("API key was lost during update")
		}
	})

	// Test Delete
	t.Run("Delete provider", func(t *testing.T) {
		err := repo.Delete(ctx, "test-anthropic-1")
		if err != nil {
			t.Fatalf("Failed to delete provider: %v", err)
		}

		_, err = repo.Get(ctx, "test-anthropic-1")
		if err == nil {
			t.Errorf("Expected error when retrieving deleted provider")
		}
	})

	// Test validation errors
	t.Run("Validation errors", func(t *testing.T) {
		tests := []struct {
			name     string
			provider *AIProvider
			wantErr  bool
		}{
			{
				name: "Missing ID",
				provider: &AIProvider{
					Name:     "Test",
					Provider: AIProviderOpenAI,
					APIKey:   "sk-test",
				},
				wantErr: true,
			},
			{
				name: "Missing name",
				provider: &AIProvider{
					ID:       "test-id",
					Provider: AIProviderOpenAI,
					APIKey:   "sk-test",
				},
				wantErr: true,
			},
			{
				name: "Missing API key for OpenAI",
				provider: &AIProvider{
					ID:       "test-id",
					Name:     "Test",
					Provider: AIProviderOpenAI,
				},
				wantErr: true,
			},
			{
				name: "Missing base URL for Ollama",
				provider: &AIProvider{
					ID:       "test-id",
					Name:     "Test",
					Provider: AIProviderOllama,
				},
				wantErr: true,
			},
			{
				name: "Invalid temperature",
				provider: &AIProvider{
					ID:          "test-id",
					Name:        "Test",
					Provider:    AIProviderOpenAI,
					APIKey:      "sk-test",
					Temperature: 3.0,
				},
				wantErr: true,
			},
			{
				name: "Invalid max tokens",
				provider: &AIProvider{
					ID:        "test-id",
					Name:      "Test",
					Provider:  AIProviderOpenAI,
					APIKey:    "sk-test",
					MaxTokens: -1,
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := repo.Create(ctx, tt.provider)
				if (err != nil) != tt.wantErr {
					t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	// Test encryption key persistence
	t.Run("Encryption key persistence", func(t *testing.T) {
		// Create a second repository instance with the same key path
		repo2, err := NewMongoAIConfigRepository(cfg)
		if err != nil {
			t.Fatalf("Failed to create second repository: %v", err)
		}
		defer repo2.Close()

		// Verify we can decrypt data encrypted by the first repository
		provider, err := repo2.Get(ctx, "test-openai-1")
		if err != nil {
			t.Fatalf("Failed to retrieve provider with second repository: %v", err)
		}

		if provider.APIKey != "sk-test-key-1234567890abcdef" {
			t.Errorf("API key decryption failed with second repository")
		}
	})
}

// TestEncryptionDecryption tests the encryption/decryption functions directly.
func TestEncryptionDecryption(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := "test-api-key-sk-1234567890abcdef"

	// Test encrypt
	ciphertext, err := encryptString(key, plaintext)
	if err != nil {
		t.Fatalf("encryptString failed: %v", err)
	}
	if ciphertext == plaintext {
		t.Error("Ciphertext should not equal plaintext")
	}

	// Test decrypt
	decrypted, err := decryptString(key, ciphertext)
	if err != nil {
		t.Fatalf("decryptString failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("Decrypted text does not match plaintext: got %s, want %s", decrypted, plaintext)
	}

	// Test decrypt with wrong key
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(i + 1)
	}
	_, err = decryptString(wrongKey, ciphertext)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key")
	}

	// Test empty string
	empty, err := encryptString(key, "")
	if err != nil {
		t.Fatalf("encryptString with empty string failed: %v", err)
	}
	if empty != "" {
		t.Error("Empty string should return empty ciphertext")
	}

	decryptedEmpty, err := decryptString(key, "")
	if err != nil {
		t.Fatalf("decryptString with empty string failed: %v", err)
	}
	if decryptedEmpty != "" {
		t.Error("Empty ciphertext should return empty plaintext")
	}
}

// TestKeyRotation tests the encryption key rotation functionality.
func TestKeyRotation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Get MongoDB URI from environment or use default
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	// Create temporary directory for test encryption keys
	tempDir := t.TempDir()

	// Create repository
	cfg := MongoAIConfigRepositoryConfig{
		MongoURI:       mongoURI,
		DatabaseName:   "healthops_test",
		CollectionName: "test_key_rotation",
		DataDir:        tempDir,
		RetentionDays:  7,
	}

	repo, err := NewMongoAIConfigRepository(cfg)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer func() {
		// Clean up test collection
		ctx := context.Background()
		repo.collection.Database().Drop(ctx)
		repo.Close()
	}()

	ctx := context.Background()

	// Create a test provider with API key
	provider := &AIProvider{
		ID:          "test-rotation-1",
		Name:        "Test Rotation Provider",
		Provider:    AIProviderOpenAI,
		APIKey:      "sk-test-key-for-rotation-123456",
		Model:       "gpt-4o",
		MaxTokens:   4096,
		Temperature: 0.7,
		Enabled:     true,
	}

	err = repo.Create(ctx, provider)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Verify initial key version is set
	retrieved, err := repo.Get(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve provider: %v", err)
	}
	initialVersion := retrieved.KeyVersion
	if initialVersion == 0 {
		t.Errorf("Expected KeyVersion to be set, got 0")
	}

	// Get current key version info before rotation
	versionsBefore := repo.GetKeyVersions()
	if len(versionsBefore) == 0 {
		t.Errorf("Expected at least one key version before rotation")
	}

	// Perform key rotation
	newKeyPath := tempDir + "/.ai_enc_key_v2"
	err = repo.RotateKey(ctx, newKeyPath)
	if err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	// Verify the provider's API key can still be decrypted after rotation
	retrievedAfter, err := repo.Get(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve provider after rotation: %v", err)
	}
	if retrievedAfter.APIKey != provider.APIKey {
		t.Errorf("API key mismatch after rotation: expected %s, got %s", provider.APIKey, retrievedAfter.APIKey)
	}

	// Verify key version was updated
	if retrievedAfter.KeyVersion <= initialVersion {
		t.Errorf("Expected KeyVersion to increase after rotation: was %d, now %d", initialVersion, retrievedAfter.KeyVersion)
	}

	// Verify key versions info updated
	versionsAfter := repo.GetKeyVersions()
	if len(versionsAfter) <= len(versionsBefore) {
		t.Errorf("Expected more key versions after rotation: before %d, after %d", len(versionsBefore), len(versionsAfter))
	}

	// Verify old key is archived
	oldKeyFound := false
	for version, info := range versionsAfter {
		if version == initialVersion {
			infoMap, ok := info.(map[string]string)
			if !ok {
				continue
			}
			if infoMap["status"] == "archived" {
				oldKeyFound = true
				break
			}
		}
	}
	if !oldKeyFound {
		t.Errorf("Expected old key version to be archived")
	}
}

// TestValidateProvider tests the provider validation logic.
func TestValidateProvider(t *testing.T) {
	// Create a mock repository for testing validation only
	repo := &MongoAIConfigRepository{}

	tests := []struct {
		name     string
		provider *AIProvider
		wantErr  bool
	}{
		{
			name: "Valid OpenAI provider",
			provider: &AIProvider{
				ID:       "test-1",
				Name:     "Test OpenAI",
				Provider: AIProviderOpenAI,
				APIKey:   "sk-test",
				Model:    "gpt-4",
			},
			wantErr: false,
		},
		{
			name: "Valid Ollama provider",
			provider: &AIProvider{
				ID:       "test-2",
				Name:     "Test Ollama",
				Provider: AIProviderOllama,
				BaseURL:  "http://localhost:11434",
				Model:    "llama2",
			},
			wantErr: false,
		},
		{
			name: "Unsupported provider",
			provider: &AIProvider{
				ID:       "test-3",
				Name:     "Test Invalid",
				Provider: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.validateProvider(tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// BenchmarkEncryptDecrypt benchmarks the encryption/decryption operations.
func BenchmarkEncryptDecrypt(b *testing.B) {
	key := make([]byte, 32)
	plaintext := "sk-test-key-1234567890abcdefghijklmnop"

	b.Run("Encrypt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = encryptString(key, plaintext)
		}
	})

	ciphertext, _ := encryptString(key, plaintext)
	b.Run("Decrypt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = decryptString(key, ciphertext)
		}
	})
}
