-- name: CreateAPIKey :one
INSERT INTO api_keys (id, user_id, name, key_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAPIKeysByUser :many
-- An owner's keys, newest first. Revoked ones stay listed: knowing a key
-- existed and ended is part of knowing who could reach the account.
SELECT * FROM api_keys
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: AuthenticateAPIKey :one
-- Resolves a bearer key to the account it acts for. Joins through users so
-- a key cannot outlive the account it belongs to.
SELECT k.id, k.user_id
FROM api_keys k
JOIN users u ON u.id = k.user_id
WHERE k.key_hash = $1
  AND k.revoked_at IS NULL
  AND u.deleted_at IS NULL;

-- name: TouchAPIKey :exec
-- Records that a key was used, at most once a minute: enough to tell a live
-- key from a forgotten one, without a write on every request.
UPDATE api_keys
SET last_used_at = now()
WHERE id = $1
  AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');

-- name: RevokeAPIKey :execrows
UPDATE api_keys
SET revoked_at = now()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;
