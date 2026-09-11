-- name: CreateAgent :one
INSERT INTO agents (id, owner_user_id, handle, display_name, description, avatar_media_id,
                    starters, private, listed, supports_schedules)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetAgentByHandle :one
SELECT * FROM agents
WHERE handle = $1 AND deleted_at IS NULL;

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
SET display_name    = COALESCE(sqlc.narg('display_name')::text, display_name),
    description     = COALESCE(sqlc.narg('description')::text, description),
    avatar_media_id = COALESCE(sqlc.narg('avatar_media_id')::uuid, avatar_media_id),
    starters        = COALESCE(sqlc.narg('starters')::jsonb, starters),
    private         = COALESCE(sqlc.narg('private')::boolean, private),
    listed          = COALESCE(sqlc.narg('listed')::boolean, listed),
    supports_schedules = COALESCE(sqlc.narg('supports_schedules')::boolean, supports_schedules),
    updated_at      = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteAgent :execrows
UPDATE agents
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCatalogue :many
-- The agents offered in the app, oldest first, so the order is the order they
-- were published in rather than whatever the planner felt like.
SELECT * FROM agents
WHERE listed AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC
LIMIT $1;
