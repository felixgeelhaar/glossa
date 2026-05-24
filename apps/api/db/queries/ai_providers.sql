-- name: CreateAIProvider :one
INSERT INTO ai_translation_providers (
  tenant_id, kind, label, base_url, model, api_key_ct, api_key_nonce, enabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, tenant_id, kind, label, base_url, model, enabled, created_at, updated_at;

-- name: ListAIProviders :many
SELECT id, tenant_id, kind, label, base_url, model, enabled, created_at, updated_at
FROM ai_translation_providers
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: ListEnabledAIProvidersForTenant :many
SELECT id, tenant_id, kind, label, base_url, model, api_key_ct, api_key_nonce
FROM ai_translation_providers
WHERE tenant_id = $1 AND enabled = TRUE
ORDER BY created_at ASC;

-- name: GetAIProvider :one
SELECT id, tenant_id, kind, label, base_url, model, api_key_ct, api_key_nonce, enabled, created_at, updated_at
FROM ai_translation_providers
WHERE id = $1;

-- name: UpdateAIProvider :exec
UPDATE ai_translation_providers
SET label = $2,
    base_url = $3,
    model = $4,
    enabled = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateAIProviderKey :exec
UPDATE ai_translation_providers
SET api_key_ct = $2,
    api_key_nonce = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteAIProvider :exec
DELETE FROM ai_translation_providers WHERE id = $1;
