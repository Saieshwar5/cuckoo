-- name: CreateBinding :one
INSERT INTO agent_bindings (id, agent_id, mode, webhook_url, secret_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActiveBinding :one
SELECT * FROM agent_bindings
WHERE agent_id = $1 AND revoked_at IS NULL;

-- name: RevokeActiveBinding :execrows
UPDATE agent_bindings
SET revoked_at = now()
WHERE agent_id = $1 AND revoked_at IS NULL;

-- name: AuthenticateBinding :one
-- Resolves a bearer secret to the agent it speaks for. Joins through agents so
-- a binding cannot outlive its agent: deleting the agent silently ends every
-- session using its secret, without a separate revocation step to forget.
SELECT b.id AS binding_id, b.agent_id, b.mode, b.status
FROM agent_bindings b
JOIN agents a ON a.id = b.agent_id
WHERE b.secret_hash = $1
  AND b.revoked_at IS NULL
  AND a.deleted_at IS NULL;
