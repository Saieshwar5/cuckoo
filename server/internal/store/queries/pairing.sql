-- name: CreatePairToken :one
INSERT INTO pair_tokens (id, token_hash, kind, agent_id, payload, max_uses, created_by_user_id, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPairTokenByHash :one
-- Resolves the token in a link. Joins the agent so a token cannot outlive
-- the agent it is for.
SELECT t.*, a.deleted_at AS agent_deleted_at
FROM pair_tokens t
JOIN agents a ON a.id = t.agent_id
WHERE t.token_hash = $1;

-- name: ListPairTokensByAgent :many
SELECT * FROM pair_tokens
WHERE agent_id = $1
ORDER BY created_at DESC, id DESC;

-- name: RevokePairToken :execrows
UPDATE pair_tokens
SET revoked_at = now()
WHERE id = $1 AND agent_id = $2 AND revoked_at IS NULL;

-- name: UsePairToken :one
-- Counts a use, if the token still has one to give. No row means it does
-- not: revoked, expired or spent, decided in the one statement so two
-- people scanning the last use of a code cannot both get in.
UPDATE pair_tokens
SET use_count = use_count + 1
WHERE id = $1
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())
  AND (max_uses IS NULL OR use_count < max_uses)
RETURNING *;

-- name: CreateContact :one
INSERT INTO contacts (user_id, agent_id, dm_conversation_id, added_via, pair_token_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetContact :one
SELECT * FROM contacts
WHERE user_id = $1 AND agent_id = $2;

-- name: ListContacts :many
-- A person's agents, newest first, with what the list shows about each.
SELECT c.*,
       a.handle,
       a.display_name,
       a.description,
       (a.avatar_media_id IS NOT NULL)::bool AS has_avatar,
       a.deleted_at   AS agent_deleted_at,
       u.display_name AS owner_display_name,
       b.status       AS binding_status
FROM contacts c
JOIN agents a ON a.id = c.agent_id
JOIN users  u ON u.id = a.owner_user_id
LEFT JOIN agent_bindings b ON b.agent_id = a.id AND b.revoked_at IS NULL
WHERE c.user_id = $1
ORDER BY c.created_at DESC, c.agent_id;

-- name: SetContactBlocked :execrows
UPDATE contacts
SET blocked_at = now()
WHERE user_id = $1 AND agent_id = $2 AND blocked_at IS NULL;

-- name: ClearContactBlocked :execrows
UPDATE contacts
SET blocked_at = NULL
WHERE user_id = $1 AND agent_id = $2 AND blocked_at IS NOT NULL;

-- name: IsConversationBlocked :one
-- Whether anyone in a conversation has blocked an agent in it. A chat with
-- a blocked agent is closed in both directions.
SELECT EXISTS (
    SELECT 1
    FROM contacts c
    JOIN participants pu ON pu.conversation_id = $1 AND pu.user_id = c.user_id
    JOIN participants pa ON pa.conversation_id = $1 AND pa.agent_id = c.agent_id
    WHERE c.blocked_at IS NOT NULL
);

-- name: GetPairToken :one
SELECT * FROM pair_tokens
WHERE id = $1 AND agent_id = $2;
