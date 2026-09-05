-- name: CreateConversation :one
INSERT INTO conversations (id, kind)
VALUES ($1, $2)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations
WHERE id = $1;

-- name: AddParticipant :exec
INSERT INTO participants (conversation_id, kind, user_id, agent_id)
VALUES ($1, $2, $3, $4);

-- name: IsUserParticipant :one
SELECT EXISTS (
    SELECT 1 FROM participants
    WHERE conversation_id = sqlc.arg('conversation_id') AND user_id = sqlc.arg('user_id')::uuid
);

-- name: ListUserConversations :many
-- The chat list, most recently active first.
--
-- Activity is the newest message, or the conversation itself when nothing has
-- been said yet. Both are UUIDv7 identifiers, so comparing them compares
-- creation times, and the newest-message lookup is one probe of the history
-- index. No denormalised last_message_at column exists to drift.
SELECT c.*
FROM conversations c
JOIN participants p ON p.conversation_id = c.id
WHERE p.user_id = sqlc.arg('user_id')::uuid
ORDER BY COALESCE(
    (SELECT m.id FROM messages m WHERE m.conversation_id = c.id ORDER BY m.id DESC LIMIT 1),
    c.id
) DESC;

-- name: ListParticipants :many
-- Members of a set of conversations with their current names. Deleted people
-- and retired agents still appear: they are part of the history, and the
-- messages they sent still name them.
SELECT p.conversation_id, p.kind, p.user_id, p.agent_id, p.joined_at,
       u.display_name AS user_display_name,
       a.display_name AS agent_display_name,
       a.handle       AS agent_handle
FROM participants p
LEFT JOIN users  u ON u.id = p.user_id
LEFT JOIN agents a ON a.id = p.agent_id
WHERE p.conversation_id = ANY(sqlc.arg('conversation_ids')::uuid[])
ORDER BY p.conversation_id, p.joined_at, p.kind, p.user_id, p.agent_id;
