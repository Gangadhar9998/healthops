# AI Encryption Key Rotation

AI provider keys are encrypted with `HEALTHOPS_AI_ENCRYPTION_KEY`. The key is
provided by the deployment secret manager and is not stored in local files.

## Rotate

1. Disable writes or schedule a short maintenance window.
2. Back up MongoDB.
3. Update `HEALTHOPS_AI_ENCRYPTION_KEY` in the secret manager.
4. Restart HealthOps.
5. Re-save AI provider credentials through the UI/API so they are encrypted
   with the new key.

The `/api/v1/ai/keys` endpoint reports key source metadata. File paths such as
`.ai_enc_key` are obsolete.
