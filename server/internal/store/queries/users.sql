-- Queries are compiled into typed Go by sqlc (`make gen`). A column that does
-- not exist, or an argument of the wrong type, is a build failure rather than a
-- runtime surprise.

-- name: CreateUser :one
INSERT INTO users (id, display_name, locale)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateUserProfile :one
-- Partial update: an argument left null leaves that column untouched, so a
-- caller changing only their name cannot accidentally blank their locale.
UPDATE users
SET display_name    = COALESCE(sqlc.narg('display_name')::text, display_name),
    locale          = COALESCE(sqlc.narg('locale')::text, locale),
    timezone        = COALESCE(sqlc.narg('timezone')::text, timezone),
    avatar_media_id = COALESCE(sqlc.narg('avatar_media_id')::uuid, avatar_media_id),
    updated_at      = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteUser :execrows
-- The account is closed and its name and face taken off: what remains in
-- other people's history reads as "Deleted account".
UPDATE users
SET deleted_at = now(), updated_at = now(), display_name = 'Deleted account', avatar_media_id = NULL
WHERE id = $1 AND deleted_at IS NULL;
