# AI Provider Encryption Key Rotation

This document describes the encryption key rotation feature for AI provider API keys stored in MongoDB.

## Overview

The MongoDB AI config repository supports encryption key rotation to enhance security by periodically updating the encryption keys used to protect API keys at rest.

## Features

- **Key Versioning**: Each encrypted API key is tagged with a key version number
- **Seamless Rotation**: Rotate keys without downtime - old keys remain decryptable
- **Safe Archival**: Previous keys are archived (not deleted) for recovery
- **API Access**: Both REST API and CLI tool for key rotation
- **Environment Variable Support**: Override default key path via `AI_ENCRYPTION_KEY_PATH`

## Architecture

### Key Version Tracking

Each AI provider document includes a `keyVersion` field:

```json
{
  "_id": "openai-prod",
  "name": "Production OpenAI",
  "provider": "openai",
  "apiKey": "<encrypted-with-key-version-2>",
  "keyVersion": 2,
  ...
}
```

### Encryption Key Configuration

The repository maintains an `EncryptionKeyConfig` structure:

```go
type EncryptionKeyConfig struct {
    currentKeyPath string           // Path to current encryption key
    previousKeys   map[int]string   // Archived keys: version -> path
    currentVersion int              // Current active version
}
```

### Key Rotation Process

1. **Generate New Key**: A new 32-byte AES-256 key is generated
2. **Re-encrypt Providers**: For each provider:
   - Decrypt API key with old key
   - Encrypt API key with new key
   - Update `keyVersion` field
   - Save to database
3. **Archive Old Key**: Old key path is stored in `previousKeys`
4. **Update Current**: New key becomes current, version increments

### Decryption Strategy

When decrypting an API key:
1. Try current key first
2. If `keyVersion` differs, try archived keys
3. Support legacy keys without version (stored before rotation feature)

## Usage

### Environment Variables

- `AI_ENCRYPTION_KEY_PATH`: Override default encryption key path
- `MONGODB_URI`: MongoDB connection string
- `MONGODB_DATABASE`: Database name (default: `healthops`)

### API Endpoints

#### List Key Versions

```bash
curl http://localhost:8080/api/v1/ai/keys
```

Response:
```json
{
  "success": true,
  "data": {
    "versions": {
      "1": {
        "path": "data/.ai_enc_key",
        "status": "archived"
      },
      "2": {
        "path": "data/.ai_enc_key_v2",
        "status": "current"
      }
    }
  }
}
```

#### Rotate Keys

```bash
curl -X POST http://localhost:8080/api/v1/ai/keys/rotate \
  -H "Content-Type: application/json" \
  -d '{"newKeyPath": "data/.ai_enc_key_v3"}'
```

Or let the system auto-generate the path:
```bash
curl -X POST http://localhost:8080/api/v1/ai/keys/rotate
```

Response:
```json
{
  "success": true,
  "data": {
    "message": "Key rotation completed successfully",
    "versions": {
      "1": {"path": "data/.ai_enc_key", "status": "archived"},
      "2": {"path": "data/.ai_enc_key_v2", "status": "archived"},
      "3": {"path": "data/.ai_enc_key_v3", "status": "current"}
    }
  }
}
```

### CLI Tool

Build the rotation utility:
```bash
cd backend
go build -o rotate-ai-keys ./cmd/rotate-ai-keys
```

List current versions:
```bash
./rotate-ai-keys -list
```

Perform rotation (auto-generates version number):
```bash
./rotate-ai-keys
```

Specify custom key path:
```bash
./rotate-ai-keys -key-path /secure/keys/ai_v4.key
```

Full options:
```bash
./rotate-ai-keys -mongo-uri "mongodb://localhost:27017" \
                 -database "healthops" \
                 -collection "healthops_ai_config" \
                 -data-dir "data"
```

## Security Considerations

### Key Storage

- Keys are stored as hex-encoded files
- File permissions are set to `0600` (owner read/write only)
- Keys are never logged or exposed in API responses

### Rotation Frequency

Recommended rotation schedule:
- **Critical environments**: Every 90 days
- **Production**: Every 180 days
- **Development/testing**: Every 365 days or as needed

### Recovery

Old keys are kept in the archive for:
1. **Rollback**: If rotation fails, old key can restore access
2. **Audit**: Investigating when a key was rotated
3. **Compliance**: Meeting retention requirements

**Important**: Do NOT delete old key files until:
- All providers have been tested with the new key
- At least one full backup cycle has completed
- You're confident no rollback will be needed

### Backup Recommendations

Before rotating keys:
1. **Backup MongoDB**: `mongodump --db healthops --collection healthops_ai_config`
2. **Backup Key Files**: Copy all `.ai_enc_key*` files to secure storage
3. **Test Restore**: Verify backups can be restored

### Troubleshooting

#### Rotation Fails Mid-Operation

The rotation process includes automatic rollback:
- If any provider fails to re-encrypt, the new key file is deleted
- Original keys remain unchanged
- Check logs for specific provider failure reasons

#### Providers Fail After Rotation

1. Check key version in provider document
2. Verify archived key files exist and are readable
3. Test decryption manually:
   ```go
   key, _ := hex.DecodeString(<key-file-content>)
   plaintext, _ := decryptString(key, <encrypted-api-key>)
   ```

#### Key File Corruption

If a key file is corrupted:
1. Restore from backup
2. If no backup exists, providers with that key version will need to have their API keys re-entered

## Implementation Details

### Thread Safety

The rotation process uses mutex locks to ensure:
- Only one rotation can occur at a time
- Encryption/decryption operations are atomic
- Key version updates are consistent

### Database Transactions

Rotation uses MongoDB transactions (when available) to ensure:
- All providers are updated atomically
- No partial updates occur
- Rollback is possible if any step fails

### Performance Impact

- Rotation is a one-time operation per provider
- Each provider requires ~10ms for re-encryption
- Rotation of 100 providers takes ~1 second
- No impact on ongoing API operations

## Testing

Run the key rotation test:
```bash
cd backend
MONGODB_URI="mongodb://localhost:27017" \
go test -v ./internal/monitoring/ai/repositories -run TestKeyRotation
```

## Migration Guide

### From File-Based Storage

If migrating from file-based AI config (`data/ai_config.json`):

1. **Set up MongoDB**:
   ```bash
   export MONGODB_URI="mongodb://localhost:27017"
   ```

2. **Existing keys will be marked with KeyVersion: 0** (legacy)
   - They'll be decrypted with the current key
   - First rotation will upgrade them to KeyVersion: 1

3. **No manual migration required** - the system handles legacy keys automatically

### From Single Key to Versioned Keys

When upgrading to the versioned key system:

1. Existing providers have `KeyVersion: 0` (implicit)
2. First rotation sets them to `KeyVersion: 1`
3. No manual intervention needed

## API Reference

### Repository Methods

```go
// RotateKey performs key rotation
func (r *MongoAIConfigRepository) RotateKey(ctx context.Context, newKeyPath string) error

// GetKeyVersions returns information about all key versions
func (r *MongoAIConfigRepository) GetKeyVersions() map[int]interface{}

// getCurrentKeyVersion returns the current active version
func (r *MongoAIConfigRepository) getCurrentKeyVersion() int

// decryptWithKeyVersion decrypts using a specific archived key
func (r *MongoAIConfigRepository) decryptWithKeyVersion(cipherHex string, version int) (string, error)
```

### Configuration

```go
type MongoAIConfigRepositoryConfig struct {
    MongoURI       string
    DatabaseName   string
    CollectionName string
    DataDir        string
    RetentionDays  int
    KeyPath        string // Optional: override default key path
}
```

## Summary

The encryption key rotation feature provides:
- ✅ Secure key lifecycle management
- ✅ Zero-downtime rotation
- ✅ Automatic rollback on failure
- ✅ Key versioning and archiving
- ✅ API and CLI interfaces
- ✅ Backward compatibility with legacy keys

For questions or issues, refer to the test suite in `internal/monitoring/ai/repositories/mongo_config_repository_test.go`.
