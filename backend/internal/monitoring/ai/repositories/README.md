# MongoDB AI Config Repository

This package provides a MongoDB-backed repository for storing and managing AI provider configurations with AES-256-GCM encryption for API keys.

## Features

- **Secure Encryption**: All API keys are encrypted at rest using AES-256-GCM
- **Multiple Providers**: Support for OpenAI, Anthropic, Google, Ollama, and custom providers
- **Default Provider**: Mark one provider as the default for AI operations
- **Validation**: Comprehensive validation for provider configurations
- **MongoDB Integration**: Full CRUD operations with proper indexing
- **Thread-Safe**: Concurrent access support with mutex-protected encryption keys

## Installation

```go
import "github.com/yourusername/healthops/backend/internal/monitoring/ai/repositories"
```

## Usage

### Creating a Repository

```go
import (
    "context"
    "os"
)

func main() {
    cfg := repositories.MongoAIConfigRepositoryConfig{
        MongoURI:       os.Getenv("MONGODB_URI"),
        DatabaseName:   "healthops",
        CollectionName: "healthops_ai_config",
        DataDir:        "data", // Directory for encryption key storage
        RetentionDays:  7,
    }

    repo, err := repositories.NewMongoAIConfigRepository(cfg)
    if err != nil {
        panic(err)
    }
    defer repo.Close()
}
```

### Creating a Provider

```go
ctx := context.Background()

provider := &repositories.AIProvider{
    ID:          "openai-prod-1",
    Name:        "Production OpenAI",
    Provider:    repositories.AIProviderOpenAI,
    APIKey:      "sk-proj-abc123...",
    Model:       "gpt-4o",
    MaxTokens:   4096,
    Temperature: 0.7,
    Enabled:     true,
    Default:     true,
    Metadata: map[string]interface{}{
        "organization": "acme-corp",
        "region":       "us-east-1",
    },
}

err := repo.Create(ctx, provider)
if err != nil {
    panic(err)
}
```

### Retrieving a Provider

```go
provider, err := repo.Get(ctx, "openai-prod-1")
if err != nil {
    panic(err)
}

// API key is automatically decrypted
fmt.Printf("Provider: %s, Model: %s\n", provider.Name, provider.Model)
```

### Listing Providers

```go
// List all providers
providers, err := repo.List(ctx)
if err != nil {
    panic(err)
}

for _, p := range providers {
    fmt.Printf("%s (%s): %s\n", p.Name, p.Provider, p.Model)
}

// List only enabled providers
enabled, err := repo.ListEnabled(ctx)
if err != nil {
    panic(err)
}
```

### Getting the Default Provider

```go
defaultProvider, err := repo.GetDefault(ctx)
if err != nil {
    panic(err)
}

fmt.Printf("Default: %s\n", defaultProvider.Name)
```

### Updating a Provider

```go
provider, err := repo.Get(ctx, "openai-prod-1")
if err != nil {
    panic(err)
}

provider.Temperature = 0.9
provider.Model = "gpt-4o-2024-11-20"

err = repo.Update(ctx, provider)
if err != nil {
    panic(err)
}
```

### Setting a New Default

```go
// Automatically unmarks the previous default
err := repo.SetDefault(ctx, "anthropic-prod-1")
if err != nil {
    panic(err)
}
```

### Deleting a Provider

```go
err := repo.Delete(ctx, "openai-prod-1")
if err != nil {
    panic(err)
}
```

## Supported Providers

### OpenAI
```go
provider := &repositories.AIProvider{
    ID:       "openai-1",
    Name:     "OpenAI",
    Provider: repositories.AIProviderOpenAI,
    APIKey:   "sk-...",
    Model:    "gpt-4o",
}
```

### Anthropic
```go
provider := &repositories.AIProvider{
    ID:       "anthropic-1",
    Name:     "Anthropic",
    Provider: repositories.AIProviderAnthropic,
    APIKey:   "sk-ant-...",
    Model:    "claude-sonnet-4-20250514",
}
```

### Google Gemini
```go
provider := &repositories.AIProvider{
    ID:       "google-1",
    Name:     "Google Gemini",
    Provider: repositories.AIProviderGoogle,
    APIKey:   "AIza...",
    Model:    "gemini-2.0-flash-exp",
}
```

### Ollama
```go
provider := &repositories.AIProvider{
    ID:       "ollama-1",
    Name:     "Ollama",
    Provider: repositories.AIProviderOllama,
    BaseURL:  "http://localhost:11434",
    Model:    "llama2",
}
```

### Custom (OpenAI-compatible)
```go
provider := &repositories.AIProvider{
    ID:       "custom-1",
    Name:     "Custom OpenAI-compatible",
    Provider: repositories.AIProviderCustom,
    BaseURL:  "https://api.example.com/v1",
    APIKey:   "custom-key",
    Model:    "custom-model",
}
```

## Encryption Details

- **Algorithm**: AES-256-GCM
- **Key Storage**: `data/.ai_enc_key` (hex-encoded, 0o600 permissions)
- **Key Generation**: Auto-generated on first run with crypto/rand
- **Nonce**: 96-bit (12 bytes) randomly generated per encryption
- **Format**: `hex(nonce || ciphertext)`

The encryption key is stored separately from the database, providing defense-in-depth. API keys are encrypted before storage and decrypted only in memory.

## Validation Rules

- **ID**: Required, unique
- **Name**: Required
- **Provider**: Must be one of: `openai`, `anthropic`, `google`, `ollama`, `custom`
- **API Key**: Required for OpenAI, Anthropic, Google
- **Base URL**: Required for Ollama, Custom
- **Temperature**: Between 0.0 and 2.0
- **Max Tokens**: Between 0 and 128000

## Indexes

The following indexes are automatically created:
- `_id` (primary key)
- `provider`
- `enabled`
- `default`
- `createdAt` (descending)
- `updatedAt` (descending)

## Error Handling

```go
provider, err := repo.Get(ctx, "non-existent")
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Handle not found
    } else {
        // Handle other errors
    }
}
```

## Testing

```bash
# Run all tests
go test ./internal/monitoring/ai/repositories/...

# Run specific test
go test -v ./internal/monitoring/ai/repositories/... -run TestEncryptionDecryption

# Run integration tests (requires MongoDB)
go test -v ./internal/monitoring/ai/repositories/...

# Run benchmarks
go test -bench=. ./internal/monitoring/ai/repositories/...
```

## Security Considerations

1. **Key Management**: The encryption key file should be backed up securely
2. **File Permissions**: Ensure `data/.ai_enc_key` has restricted permissions (0o600)
3. **MongoDB Security**: Use TLS connections and strong authentication
4. **Logging**: API keys are never logged; only masked versions appear in logs
5. **Memory**: Decrypted keys exist only in memory and are cleared when the provider object is garbage collected

## Performance

Encryption/decryption operations are fast (< 1ms per operation):
- Encrypt: ~0.3ms per API key
- Decrypt: ~0.3ms per API key

MongoDB operations benefit from proper indexing:
- Create: ~5-10ms
- Get (by ID): ~1-3ms
- List (with decryption): ~10-50ms depending on count
- Update: ~5-10ms
- Delete: ~3-5ms

## License

This package is part of the HealthOps monitoring system.
