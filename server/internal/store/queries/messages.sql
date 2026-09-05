-- name: CreateMessage :one
INSERT INTO messages (id, conversation_id, sender_kind, sender_user_id, sender_agent_id, body, idempotency_key, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetMessage :one
SELECT * FROM messages
WHERE id = $1;

-- name: FinishMessage :one
-- Ends a stream: the whole text lands in one update. Scoped to the agent
-- that started it, and to a message still streaming, so a second finish or
-- another agent's finish changes nothing.
UPDATE messages
SET body = sqlc.arg('body'), status = 'complete', truncated = sqlc.arg('truncated')
WHERE id = sqlc.arg('id')
  AND sender_agent_id = sqlc.arg('sender_agent_id')::uuid
  AND status = 'streaming'
RETURNING *;

-- name: ListStaleStreamingMessages :many
-- Streams still open long after they began: the buffer's own bookkeeping
-- was lost, and the row must be finished from whatever is left.
SELECT * FROM messages
WHERE status = 'streaming' AND created_at < sqlc.arg('started_before')::timestamptz
ORDER BY created_at
LIMIT 100;

-- name: GetMessageByUserKey :one
SELECT * FROM messages
WHERE sender_user_id = sqlc.arg('sender_user_id')::uuid AND idempotency_key = sqlc.arg('idempotency_key')::text;

-- name: GetMessageByAgentKey :one
SELECT * FROM messages
WHERE sender_agent_id = sqlc.arg('sender_agent_id')::uuid AND idempotency_key = sqlc.arg('idempotency_key')::text;

-- name: ListMessagesBeforeForAgent :many
-- History as an agent sees it: only from the moment it joined. An agent
-- added to a group later must not be handed everything said before it.
SELECT m.* FROM messages m
JOIN participants p ON p.conversation_id = m.conversation_id AND p.agent_id = sqlc.arg('agent_id')::uuid
WHERE m.conversation_id = sqlc.arg('conversation_id')
  AND m.created_at >= p.joined_at
  AND (sqlc.narg('before')::uuid IS NULL OR m.id < sqlc.narg('before')::uuid)
ORDER BY m.id DESC
LIMIT sqlc.arg('page_size');

-- name: ListMessagesBefore :many
-- One page of history, newest first. The cursor is a message id: everything
-- older than it, or the newest page when it is null.
SELECT * FROM messages
WHERE conversation_id = sqlc.arg('conversation_id')
  AND (sqlc.narg('before')::uuid IS NULL OR id < sqlc.narg('before')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('page_size');

-- name: ListLatestMessages :many
-- The newest message of each conversation in the set, for chat-list previews.
SELECT DISTINCT ON (conversation_id) *
FROM messages
WHERE conversation_id = ANY(sqlc.arg('conversation_ids')::uuid[])
ORDER BY conversation_id, id DESC;

-- name: ListMessagesByIDs :many
SELECT * FROM messages
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListMessagesAfter :many
-- Catching up: everything newer than a message the caller already has,
-- oldest first, so a client that was away fills its gap in order.
SELECT * FROM messages
WHERE conversation_id = sqlc.arg('conversation_id')
  AND id > sqlc.arg('after')::uuid
ORDER BY id ASC
LIMIT sqlc.arg('page_size');
