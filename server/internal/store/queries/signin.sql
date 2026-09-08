-- name: CreateIdentity :one
INSERT INTO user_identities (id, user_id, kind, value, verified_at)
VALUES ($1, $2, $3, $4, now())
RETURNING *;

-- name: GetIdentity :one
SELECT * FROM user_identities
WHERE kind = $1 AND value = $2;

-- name: CreateSignInCode :one
INSERT INTO sign_in_codes (id, email, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLatestSignInCode :one
-- Only the newest code for an address counts; requesting another retires
-- the last one.
SELECT * FROM sign_in_codes
WHERE email = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountSignInAttempt :one
UPDATE sign_in_codes
SET attempts = attempts + 1
WHERE id = $1
RETURNING attempts;

-- name: MarkSignInCodeUsed :exec
UPDATE sign_in_codes
SET used_at = now()
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, device_name, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: AuthenticateSession :one
-- Resolves a bearer token to its person. Joins through users so a session
-- cannot outlive the account. last_seen_at comes back so the caller can
-- decide whether it is stale enough to be worth a write.
SELECT s.id, s.user_id, s.last_seen_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND u.deleted_at IS NULL;

-- name: RevokeSession :execrows
UPDATE sessions
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :execrows
-- Every device of one person: signing out everywhere, and what deleting an
-- account does on the way out.
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeOtherUserSessions :execrows
-- Signing out every device but the one asking.
UPDATE sessions
SET revoked_at = now()
WHERE user_id = sqlc.arg('user_id') AND id <> sqlc.arg('keep') AND revoked_at IS NULL;

-- name: ListUserSessions :many
-- The devices a person is signed in on, newest first.
SELECT id, device_name, last_seen_at, created_at, expires_at
FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- name: RevokeUserSession :execrows
-- One device, ended by its owner. Scoped to the person so an id learned
-- from somewhere else is not enough.
UPDATE sessions
SET revoked_at = now()
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id') AND revoked_at IS NULL;

-- name: TouchSession :exec
-- Records that a session is in use, at most as often as the caller asks.
UPDATE sessions
SET last_seen_at = now()
WHERE id = $1;

-- name: RevokeOldestUserSessions :execrows
-- Holds a person to a number of devices: past it, the ones that have gone
-- longest without being used are ended first, then the oldest.
UPDATE sessions
SET revoked_at = now()
WHERE id IN (
    SELECT older.id FROM sessions older
    WHERE older.user_id = sqlc.arg('user_id') AND older.revoked_at IS NULL AND older.expires_at > now()
    -- The id breaks a tie. Two sessions can share a timestamp — they are
    -- written by now(), which is the transaction's clock, not the statement's
    -- — and without a second term the row that ends is whichever one Postgres
    -- happened to return. Identifiers are UUIDv7, so ordering by one is
    -- ordering by when it was made.
    ORDER BY COALESCE(older.last_seen_at, older.created_at) DESC, older.id DESC
    OFFSET sqlc.arg('keep')
);

-- name: DeleteIdentities :exec
-- Cut the ways in. The address is free to open a fresh account afterwards.
DELETE FROM user_identities WHERE user_id = $1;
