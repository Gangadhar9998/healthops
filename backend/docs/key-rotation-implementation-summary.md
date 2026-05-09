# AI Key Rotation Summary

AI provider API keys are encrypted in MongoDB with `HEALTHOPS_AI_ENCRYPTION_KEY`.
That key is deployment-managed, not stored in `backend/data` and not rotated by a
local CLI.

## Current Behavior

- `HEALTHOPS_AI_ENCRYPTION_KEY` is required at service startup.
- MongoDB stores encrypted provider credentials.
- `GET /api/v1/ai/keys` reports key metadata.
- `POST /api/v1/ai/keys/rotate` returns a clear not-implemented response because
  live key rotation must happen through the deployment secret manager.

## Rotation Procedure

1. Back up MongoDB and the deployment secret store.
2. Generate a new `HEALTHOPS_AI_ENCRYPTION_KEY`.
3. Update the deployment secret and restart HealthOps.
4. Re-save AI provider credentials through the API/UI so they are encrypted with
   the new key.
5. Verify AI analysis works, then retire the old secret according to your secret
   manager policy.

Legacy local key files such as `.ai_enc_key` and the old `rotate-ai-keys` CLI are
obsolete in the MongoDB-only architecture.
