-- Users table — admins + translators, scoped by tenant_id via RLS.
-- All reads/writes assume `SET LOCAL app.current_tenant` is active
-- (or that the caller has BYPASSRLS, which only the migration
-- container does); cross-tenant access fails at the DB layer.

-- name: CreateUser :one
INSERT INTO users (tenant_id, email, password_hash, role, locales)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, email, role, locales, created_at;

-- name: GetUserByEmail :one
SELECT id, tenant_id, email, password_hash, role, locales, created_at
FROM users
WHERE tenant_id = $1 AND email = $2;

-- name: GetUserByID :one
SELECT id, tenant_id, email, password_hash, role, locales, created_at
FROM users
WHERE id = $1;

-- name: ListUsersForTenant :many
SELECT id, tenant_id, email, role, locales, created_at
FROM users
WHERE tenant_id = $1
ORDER BY created_at;

-- name: UpdateUserLocales :exec
UPDATE users SET locales = $2 WHERE id = $1;

-- name: UpdateUserPasswordHash :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: CountAdminsInTenant :one
SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND role = 'admin';
