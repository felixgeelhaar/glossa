-- name: CreateTenant :one
INSERT INTO tenants (slug, name)
VALUES ($1, $2)
RETURNING id, slug, name, created_at;

-- name: GetTenantBySlug :one
SELECT id, slug, name, created_at
FROM tenants
WHERE slug = $1;

-- name: GetTenantByID :one
SELECT id, slug, name, created_at
FROM tenants
WHERE id = $1;
