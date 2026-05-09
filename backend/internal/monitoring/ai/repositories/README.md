# MongoDB AI Config Repository

This package stores AI provider configuration in MongoDB. Provider API keys are
encrypted with AES-256-GCM using the required `HEALTHOPS_AI_ENCRYPTION_KEY`
environment secret.

## Responsibilities

- CRUD for OpenAI, Anthropic, Google, Ollama, and custom provider configs.
- Default-provider selection.
- Validation for provider-specific required fields.
- MongoDB indexes for provider, enabled/default flags, and update timestamps.
- Encryption/decryption of API keys in memory at repository boundaries.

## Usage

```go
cfg := repositories.MongoAIConfigRepositoryConfig{
    MongoURI:       os.Getenv("MONGODB_URI"),
    DatabaseName:   os.Getenv("MONGODB_DATABASE"),
    CollectionName: os.Getenv("MONGODB_COLLECTION_PREFIX") + "_ai_config",
    RetentionDays:  7,
}

repo, err := repositories.NewMongoAIConfigRepository(cfg)
if err != nil {
    panic(err)
}
defer repo.Close()
```

`HEALTHOPS_AI_ENCRYPTION_KEY` must be present and at least 32 bytes before the
repository is created.

## Key Rotation

Runtime key rotation is intentionally disabled. Rotate through the deployment
secret manager:

1. Back up MongoDB and deployment secrets.
2. Replace `HEALTHOPS_AI_ENCRYPTION_KEY`.
3. Restart HealthOps.
4. Re-save AI provider credentials so they are encrypted with the new key.

Local key files such as `data/.ai_enc_key` are obsolete.
