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
-- cannot outlive the account.
SELECT s.id, s.user_id
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
-- One device per person at launch: signing in anywhere signs out everywhere
-- else. Enforced here rather than in the schema, so multi-device is a rule
-- change and not a migration.
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
