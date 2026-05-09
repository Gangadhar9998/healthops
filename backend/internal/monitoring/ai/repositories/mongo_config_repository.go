package repositories

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// AIProviderType identifies supported AI providers.
type AIProviderType string

const (
	AIProviderOpenAI    AIProviderType = "openai"
	AIProviderAnthropic AIProviderType = "anthropic"
	AIProviderGoogle    AIProviderType = "google"
	AIProviderOllama    AIProviderType = "ollama"
	AIProviderCustom    AIProviderType = "custom"
)

// AIProvider represents an AI provider configuration with encrypted API key.
type AIProvider struct {
	ID          string         `json:"id" bson:"_id"`
	Name        string         `json:"name" bson:"name"`
	Provider    AIProviderType `json:"provider" bson:"provider"`
	BaseURL     string         `json:"baseUrl,omitempty" bson:"baseUrl,omitempty"`
	APIKey      string         `json:"-" bson:"apiKey"` // encrypted at rest
	Model       string         `json:"model,omitempty" bson:"model,omitempty"`
	MaxTokens   int            `json:"maxTokens,omitempty" bson:"maxTokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty" bson:"temperature,omitempty"`
	Enabled     bool           `json:"enabled" bson:"enabled"`
	Default     bool           `json:"default" bson:"default"`
	CreatedAt   time.Time      `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt" bson:"updatedAt"`
	// KeyVersion tracks which encryption key version encrypted this API key
	KeyVersion int `json:"keyVersion" bson:"keyVersion"`
	// Metadata allows storing provider-specific configuration
	Metadata map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

// AIConfigRepository defines the interface for AI provider configuration persistence.
type AIConfigRepository interface {
	// Create adds a new AI provider configuration.
	Create(ctx context.Context, provider *AIProvider) error

	// Get retrieves an AI provider by ID.
	Get(ctx context.Context, id string) (*AIProvider, error)

	// List returns all AI providers (with decrypted keys).
	List(ctx context.Context) ([]*AIProvider, error)

	// ListEnabled returns only enabled providers.
	ListEnabled(ctx context.Context) ([]*AIProvider, error)

	// GetDefault returns the provider marked as default.
	GetDefault(ctx context.Context) (*AIProvider, error)

	// Update modifies an existing provider configuration.
	Update(ctx context.Context, provider *AIProvider) error

	// Delete removes a provider configuration.
	Delete(ctx context.Context, id string) error

	// SetDefault marks a provider as the default (unmarks others).
	SetDefault(ctx context.Context, id string) error

	// Close closes the repository and releases resources.
	Close() error
}

// EncryptionKeyConfig manages encryption key versioning for rotation.
type EncryptionKeyConfig struct {
	mu             sync.RWMutex
	currentKeyPath string
	previousKeys   map[int]string // key version -> key path
	currentVersion int
}

// MongoAIConfigRepository implements AIConfigRepository with MongoDB backend.
type MongoAIConfigRepository struct {
	client        *mongo.Client
	db            *mongo.Database
	collection    *mongo.Collection
	encKey        []byte
	encKeyMutex   sync.RWMutex
	keyConfig     *EncryptionKeyConfig
	retentionDays int
}

// MongoAIConfigRepositoryConfig holds configuration for the repository.
type MongoAIConfigRepositoryConfig struct {
	MongoURI       string
	DatabaseName   string
	CollectionName string
	DataDir        string // Deprecated; encryption key comes from HEALTHOPS_AI_ENCRYPTION_KEY.
	RetentionDays  int
	// KeyPath is deprecated and retained only for API compatibility.
	KeyPath string
}

// NewMongoAIConfigRepository creates a new MongoDB-backed AI config repository.
func NewMongoAIConfigRepository(cfg MongoAIConfigRepositoryConfig) (*MongoAIConfigRepository, error) {
	if cfg.MongoURI == "" {
		return nil, errors.New("mongo uri is required")
	}
	if cfg.DatabaseName == "" {
		cfg.DatabaseName = "healthops"
	}
	if cfg.CollectionName == "" {
		cfg.CollectionName = "healthops_ai_config"
	}
	// Force IPv4: replace localhost with 127.0.0.1 to avoid IPv6 socket issues on macOS
	uri := strings.ReplaceAll(cfg.MongoURI, "localhost", "127.0.0.1")

	// Configure client options with longer timeouts for remote connections
	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetMaxPoolSize(100).
		SetWriteConcern(writeconcern.W1())

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect failed: %w", err)
	}

	// Ping with timeout to verify connection
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping failed: %w", err)
	}

	// Initialize encryption key configuration
	keyConfig := &EncryptionKeyConfig{
		currentKeyPath: "env:HEALTHOPS_AI_ENCRYPTION_KEY",
		previousKeys:   make(map[int]string),
		currentVersion: 1, // Start at version 1
	}

	repo := &MongoAIConfigRepository{
		client:        client,
		db:            client.Database(cfg.DatabaseName),
		collection:    client.Database(cfg.DatabaseName).Collection(cfg.CollectionName),
		keyConfig:     keyConfig,
		retentionDays: cfg.RetentionDays,
	}

	// Load or create encryption key
	encKey, err := repo.loadOrCreateEncKey()
	if err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("init encryption key: %w", err)
	}
	repo.encKey = encKey

	indexCtx, indexCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer indexCancel()

	if err := repo.ensureIndexes(indexCtx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("create ai config indexes: %w", err)
	}

	return repo, nil
}

// ensureIndexes creates necessary indexes for the AI config collection.
func (r *MongoAIConfigRepository) ensureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "provider", Value: 1}}},
		{Keys: bson.D{{Key: "enabled", Value: 1}}},
		{Keys: bson.D{{Key: "default", Value: 1}}},
		{Keys: bson.D{{Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "updatedAt", Value: -1}}},
	})
	return err
}

// loadOrCreateEncKey loads the required encryption key from the environment.
func (r *MongoAIConfigRepository) loadOrCreateEncKey() ([]byte, error) {
	r.encKeyMutex.Lock()
	defer r.encKeyMutex.Unlock()

	r.keyConfig.mu.Lock()
	defer r.keyConfig.mu.Unlock()

	key, err := normalizeEncryptionKey(os.Getenv("HEALTHOPS_AI_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return key, nil
}

// encryptString encrypts a plaintext string using AES-256-GCM.
func (r *MongoAIConfigRepository) encryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	r.encKeyMutex.RLock()
	key := r.encKey
	r.encKeyMutex.RUnlock()

	return encryptString(key, plaintext)
}

// decryptString decrypts a hex-encoded ciphertext string using AES-256-GCM.
func (r *MongoAIConfigRepository) decryptString(cipherHex string) (string, error) {
	if cipherHex == "" {
		return "", nil
	}

	r.encKeyMutex.RLock()
	key := r.encKey
	r.encKeyMutex.RUnlock()

	return decryptString(key, cipherHex)
}

// encryptProvider encrypts the API key in the provider before storage.
func (r *MongoAIConfigRepository) encryptProvider(provider *AIProvider) error {
	if provider.APIKey != "" {
		encrypted, err := r.encryptString(provider.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt api key for %s: %w", provider.ID, err)
		}
		provider.APIKey = encrypted
		// Set the key version to current version
		r.keyConfig.mu.RLock()
		provider.KeyVersion = r.keyConfig.currentVersion
		r.keyConfig.mu.RUnlock()
	}
	return nil
}

// decryptProvider decrypts the API key after retrieval from storage.
func (r *MongoAIConfigRepository) decryptProvider(provider *AIProvider) error {
	if provider.APIKey != "" {
		// Try current key first
		decrypted, err := r.decryptString(provider.APIKey)
		if err == nil {
			provider.APIKey = decrypted
			return nil
		}

		// If current key fails, try previous keys if KeyVersion > 0
		if provider.KeyVersion > 0 && provider.KeyVersion != r.getCurrentKeyVersion() {
			decrypted, err = r.decryptWithKeyVersion(provider.APIKey, provider.KeyVersion)
			if err == nil {
				provider.APIKey = decrypted
				return nil
			}
		}

		// If all decryption attempts fail, the key might be stored in plaintext (legacy data)
		// We'll log a warning but continue with the encrypted value
		fmt.Printf("WARNING: Failed to decrypt API key for %s (version %d): %v\n", provider.ID, provider.KeyVersion, err)
		return fmt.Errorf("decrypt api key for %s: %w", provider.ID, err)
	}
	return nil
}

// Create adds a new AI provider configuration.
func (r *MongoAIConfigRepository) Create(ctx context.Context, provider *AIProvider) error {
	if provider.ID == "" {
		return errors.New("provider ID is required")
	}

	if err := r.validateProvider(provider); err != nil {
		return err
	}

	// Check if provider already exists
	exists, err := r.collection.CountDocuments(ctx, bson.M{"_id": provider.ID})
	if err != nil {
		return fmt.Errorf("check existing provider: %w", err)
	}
	if exists > 0 {
		return fmt.Errorf("provider with ID %s already exists", provider.ID)
	}

	// If this is marked as default, unmark existing default
	if provider.Default {
		if err := r.clearDefault(ctx); err != nil {
			return fmt.Errorf("clear existing default: %w", err)
		}
	}

	// Set timestamps
	now := time.Now().UTC()
	provider.CreatedAt = now
	provider.UpdatedAt = now

	// Encrypt API key before storage
	providerCopy := *provider
	if err := r.encryptProvider(&providerCopy); err != nil {
		return err
	}

	_, err = r.collection.InsertOne(ctx, providerCopy)
	if err != nil {
		return fmt.Errorf("insert provider: %w", err)
	}

	return nil
}

// Get retrieves an AI provider by ID and decrypts its API key.
func (r *MongoAIConfigRepository) Get(ctx context.Context, id string) (*AIProvider, error) {
	if id == "" {
		return nil, errors.New("provider ID is required")
	}

	var provider AIProvider
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&provider)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("provider not found: %s", id)
		}
		return nil, fmt.Errorf("find provider: %w", err)
	}

	// Decrypt API key
	if err := r.decryptProvider(&provider); err != nil {
		return nil, err
	}

	return &provider, nil
}

// List returns all AI providers with decrypted keys.
func (r *MongoAIConfigRepository) List(ctx context.Context) ([]*AIProvider, error) {
	cursor, err := r.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("find providers: %w", err)
	}
	defer cursor.Close(ctx)

	var providers []*AIProvider
	for cursor.Next(ctx) {
		var provider AIProvider
		if err := cursor.Decode(&provider); err != nil {
			return nil, fmt.Errorf("decode provider: %w", err)
		}

		// Decrypt API key
		if err := r.decryptProvider(&provider); err != nil {
			// Skip providers with decryption errors
			continue
		}

		providers = append(providers, &provider)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return providers, nil
}

// ListEnabled returns only enabled providers with decrypted keys.
func (r *MongoAIConfigRepository) ListEnabled(ctx context.Context) ([]*AIProvider, error) {
	filter := bson.M{"enabled": true}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("find enabled providers: %w", err)
	}
	defer cursor.Close(ctx)

	var providers []*AIProvider
	for cursor.Next(ctx) {
		var provider AIProvider
		if err := cursor.Decode(&provider); err != nil {
			return nil, fmt.Errorf("decode provider: %w", err)
		}

		// Decrypt API key
		if err := r.decryptProvider(&provider); err != nil {
			// Skip providers with decryption errors
			continue
		}

		providers = append(providers, &provider)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return providers, nil
}

// GetDefault returns the provider marked as default.
func (r *MongoAIConfigRepository) GetDefault(ctx context.Context) (*AIProvider, error) {
	var provider AIProvider
	err := r.collection.FindOne(ctx, bson.M{"default": true}).Decode(&provider)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("no default provider configured")
		}
		return nil, fmt.Errorf("find default provider: %w", err)
	}

	// Decrypt API key
	if err := r.decryptProvider(&provider); err != nil {
		return nil, err
	}

	return &provider, nil
}

// Update modifies an existing provider configuration.
func (r *MongoAIConfigRepository) Update(ctx context.Context, provider *AIProvider) error {
	if provider.ID == "" {
		return errors.New("provider ID is required")
	}

	if err := r.validateProvider(provider); err != nil {
		return err
	}

	// Check if provider exists
	exists, err := r.collection.CountDocuments(ctx, bson.M{"_id": provider.ID})
	if err != nil {
		return fmt.Errorf("check existing provider: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("provider not found: %s", provider.ID)
	}

	// If this is marked as default, unmark existing default
	if provider.Default {
		if err := r.clearDefault(ctx); err != nil {
			return fmt.Errorf("clear existing default: %w", err)
		}
	}

	// Update timestamp
	provider.UpdatedAt = time.Now().UTC()

	// Encrypt API key before storage
	providerCopy := *provider
	if err := r.encryptProvider(&providerCopy); err != nil {
		return err
	}

	update := bson.M{"$set": providerCopy}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": provider.ID}, update)
	if err != nil {
		return fmt.Errorf("update provider: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("provider not found: %s", provider.ID)
	}

	return nil
}

// Delete removes a provider configuration.
func (r *MongoAIConfigRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("provider ID is required")
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}

	return nil
}

// SetDefault marks a provider as the default (unmarks others).
func (r *MongoAIConfigRepository) SetDefault(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("provider ID is required")
	}

	// Check if provider exists
	exists, err := r.collection.CountDocuments(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("check existing provider: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}

	// Clear existing default
	if err := r.clearDefault(ctx); err != nil {
		return fmt.Errorf("clear existing default: %w", err)
	}

	// Set new default
	update := bson.M{"$set": bson.M{"default": true, "updatedAt": time.Now().UTC()}}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return fmt.Errorf("set default: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}

	return nil
}

// clearDefault removes the default flag from all providers.
func (r *MongoAIConfigRepository) clearDefault(ctx context.Context) error {
	update := bson.M{"$set": bson.M{"default": false, "updatedAt": time.Now().UTC()}}
	_, err := r.collection.UpdateMany(ctx, bson.M{"default": true}, update)
	return err
}

// validateProvider validates the provider configuration.
func (r *MongoAIConfigRepository) validateProvider(provider *AIProvider) error {
	if provider.Name == "" {
		return errors.New("provider name is required")
	}

	switch provider.Provider {
	case AIProviderOpenAI, AIProviderAnthropic, AIProviderGoogle:
		if provider.APIKey == "" {
			return fmt.Errorf("API key is required for %s provider", provider.Provider)
		}
	case AIProviderOllama, AIProviderCustom:
		if provider.BaseURL == "" {
			return fmt.Errorf("base URL is required for %s provider", provider.Provider)
		}
	default:
		return fmt.Errorf("unsupported provider: %s (supported: openai, anthropic, google, ollama, custom)", provider.Provider)
	}

	if provider.Temperature < 0 || provider.Temperature > 2.0 {
		return errors.New("temperature must be between 0.0 and 2.0")
	}
	if provider.MaxTokens < 0 || provider.MaxTokens > 128000 {
		return errors.New("maxTokens must be between 0 and 128000")
	}

	return nil
}

// Close closes the repository and releases resources.
func (r *MongoAIConfigRepository) Close() error {
	if r.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return r.client.Disconnect(ctx)
	}
	return nil
}

// Ping checks whether MongoDB is reachable.
func (r *MongoAIConfigRepository) Ping(ctx context.Context) error {
	if r.client == nil {
		return errors.New("mongo client is nil")
	}
	return r.client.Ping(ctx, nil)
}

// --- Key Version Metadata ---

// getCurrentKeyVersion returns the current encryption key version.
func (r *MongoAIConfigRepository) getCurrentKeyVersion() int {
	r.keyConfig.mu.RLock()
	defer r.keyConfig.mu.RUnlock()
	return r.keyConfig.currentVersion
}

// RotateKey is intentionally unsupported at runtime. AI encryption is backed
// by HEALTHOPS_AI_ENCRYPTION_KEY, so rotation must be performed through the
// deployment secret manager followed by a service restart and credential resave.
func (r *MongoAIConfigRepository) RotateKey(ctx context.Context, newKeyPath string) error {
	_ = ctx
	_ = newKeyPath
	return errors.New("AI key rotation is env-backed: update HEALTHOPS_AI_ENCRYPTION_KEY and restart")
}

// decryptWithKeyVersion attempts to decrypt a ciphertext using a specific key version.
func (r *MongoAIConfigRepository) decryptWithKeyVersion(cipherHex string, version int) (string, error) {
	r.keyConfig.mu.RLock()
	keyPath, exists := r.keyConfig.previousKeys[version]
	r.keyConfig.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("key version %d not found in archive", version)
	}

	key, err := normalizeEncryptionKey(keyPath)
	if err != nil {
		return "", fmt.Errorf("decode archived key: %w", err)
	}

	return decryptString(key, cipherHex)
}

// GetKeyVersions returns information about all key versions.
func (r *MongoAIConfigRepository) GetKeyVersions() map[int]interface{} {
	r.keyConfig.mu.RLock()
	defer r.keyConfig.mu.RUnlock()

	result := make(map[int]interface{})
	result[r.keyConfig.currentVersion] = map[string]string{
		"source": r.keyConfig.currentKeyPath,
		"status": "current",
	}

	for version, path := range r.keyConfig.previousKeys {
		result[version] = map[string]string{
			"source": path,
			"status": "archived",
		}
	}

	return result
}

// --- AES-256-GCM Encryption Helpers --

// encryptString encrypts a plaintext string using AES-256-GCM.
func encryptString(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// decryptString decrypts a hex-encoded ciphertext string using AES-256-GCM.
func decryptString(key []byte, cipherHex string) (string, error) {
	if cipherHex == "" {
		return "", nil
	}

	ciphertext, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

func normalizeEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("HEALTHOPS_AI_ENCRYPTION_KEY is required")
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len([]byte(raw)) < 32 {
		return nil, errors.New("HEALTHOPS_AI_ENCRYPTION_KEY must be at least 32 bytes")
	}
	if len([]byte(raw)) == 32 {
		return []byte(raw), nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}
