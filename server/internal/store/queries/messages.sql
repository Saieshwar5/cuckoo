-- name: CreateMessage :one
INSERT INTO messages (id, conversation_id, sender_kind, sender_user_id, sender_agent_id, body)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

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
