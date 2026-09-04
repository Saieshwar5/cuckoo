-- name: CreateAgent :one
INSERT INTO agents (id, owner_user_id, handle, display_name, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAgent :one
SELECT * FROM agents
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListAgentsByOwner :many
SELECT * FROM agents
WHERE owner_user_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: UpdateAgent :one
-- Partial update: a null argument leaves that column as it is.
UPDATE agents
SET display_name = COALESCE(sqlc.narg('display_name')::text, display_name),
    description  = COALESCE(sqlc.narg('description')::text, description),
    updated_at   = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteAgent :execrows
UPDATE agents
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;
