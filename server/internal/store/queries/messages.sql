-- name: CreateMessage :one
INSERT INTO messages (id, conversation_id, sender_kind, sender_user_id, sender_agent_id, body, idempotency_key, status, reply_to_message_id, signature)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateMessageBody :exec
-- Records a fact about a message after the fact: which of its buttons was
-- tapped. Nothing else about a stored message ever changes this way.
UPDATE messages
SET body = $2
WHERE id = $1;

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

-- name: SetMessageSignature :exec
-- The hub's mark, put on a streamed message once its whole text is known.
UPDATE messages SET signature = $2 WHERE id = $1;

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
-- One page of history as a person sees it, newest first. The cursor is a
-- message id: everything older than it, or the newest page when it is null.
-- What they cleared or hid is not there; it is still there for everyone
-- else.
SELECT m.* FROM messages m
JOIN participants p ON p.conversation_id = m.conversation_id AND p.user_id = sqlc.arg('user_id')::uuid
WHERE m.conversation_id = sqlc.arg('conversation_id')
  AND (p.cleared_before IS NULL OR m.id > p.cleared_before)
  AND NOT EXISTS (SELECT 1 FROM message_hides h WHERE h.user_id = p.user_id AND h.message_id = m.id)
  AND (sqlc.narg('before')::uuid IS NULL OR m.id < sqlc.narg('before')::uuid)
ORDER BY m.id DESC
LIMIT sqlc.arg('page_size');

-- name: ListLatestMessages :many
-- The newest message of each conversation in the set, for chat-list previews.
SELECT DISTINCT ON (conversation_id) *
FROM messages
WHERE conversation_id = ANY(sqlc.arg('conversation_ids')::uuid[])
ORDER BY conversation_id, id DESC;

-- name: ListLatestVisibleMessages :many
-- ListLatestMessages as one person sees it: the newest message they have
-- not cleared or hidden, so a chat-list row never previews what they put
-- out of sight.
SELECT DISTINCT ON (m.conversation_id) m.*
FROM messages m
JOIN participants p ON p.conversation_id = m.conversation_id AND p.user_id = sqlc.arg('user_id')::uuid
WHERE m.conversation_id = ANY(sqlc.arg('conversation_ids')::uuid[])
  AND (p.cleared_before IS NULL OR m.id > p.cleared_before)
  AND NOT EXISTS (SELECT 1 FROM message_hides h WHERE h.user_id = p.user_id AND h.message_id = m.id)
ORDER BY m.conversation_id, m.id DESC;

-- name: ClearConversation :execrows
-- Everything said so far goes out of this person's view. A uuid has no
-- max(), so the newest is found by order.
UPDATE participants p
SET cleared_before = (SELECT m.id FROM messages m WHERE m.conversation_id = p.conversation_id ORDER BY m.id DESC LIMIT 1)
WHERE p.conversation_id = $1 AND p.user_id = $2;

-- name: HideMessage :exec
INSERT INTO message_hides (user_id, message_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListMessagesByIDs :many
SELECT * FROM messages
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListMessagesAfter :many
-- Catching up: everything newer than a message the caller already has,
-- oldest first, so a client that was away fills its gap in order. The
-- person's view, as in ListMessagesBefore.
SELECT m.* FROM messages m
JOIN participants p ON p.conversation_id = m.conversation_id AND p.user_id = sqlc.arg('user_id')::uuid
WHERE m.conversation_id = sqlc.arg('conversation_id')
  AND (p.cleared_before IS NULL OR m.id > p.cleared_before)
  AND NOT EXISTS (SELECT 1 FROM message_hides h WHERE h.user_id = p.user_id AND h.message_id = m.id)
  AND m.id > sqlc.arg('after')::uuid
ORDER BY m.id ASC
LIMIT sqlc.arg('page_size');
