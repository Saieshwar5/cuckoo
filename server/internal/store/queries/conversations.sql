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

-- name: IsAgentParticipant :one
SELECT EXISTS (
    SELECT 1 FROM participants
    WHERE conversation_id = sqlc.arg('conversation_id') AND agent_id = sqlc.arg('agent_id')::uuid
);

-- name: ListUserConversations :many
-- The chat list, most recently active first.
--
-- Activity is the newest message, or the conversation itself when nothing has
-- been said yet. Both are UUIDv7 identifiers, so comparing them compares
-- creation times, and the newest-message lookup is one probe of the history
-- index. No denormalised last_message_at column exists to drift.
--
-- A chat with an agent the person removed from their list is not listed;
-- it is still theirs to open by id, so the history is not lost.
SELECT c.*
FROM conversations c
JOIN participants p ON p.conversation_id = c.id
WHERE p.user_id = sqlc.arg('user_id')::uuid
  AND NOT EXISTS (
      SELECT 1
      FROM participants pa
      JOIN contacts ct ON ct.agent_id = pa.agent_id AND ct.user_id = p.user_id
      WHERE pa.conversation_id = c.id AND ct.removed_at IS NOT NULL
  )
ORDER BY COALESCE(
    (SELECT m.id FROM messages m WHERE m.conversation_id = c.id ORDER BY m.id DESC LIMIT 1),
    c.id
) DESC;

-- name: FindDM :one
-- The chat between a person and an agent, if they have one.
SELECT c.*
FROM conversations c
JOIN participants pu ON pu.conversation_id = c.id AND pu.user_id = sqlc.arg('user_id')::uuid
JOIN participants pa ON pa.conversation_id = c.id AND pa.agent_id = sqlc.arg('agent_id')::uuid
WHERE c.kind = 'dm'
ORDER BY c.id
LIMIT 1;

-- name: ListParticipants :many
-- Members of a set of conversations with their current names. Deleted people
-- and retired agents still appear: they are part of the history, and the
-- messages they sent still name them.
SELECT p.conversation_id, p.kind, p.user_id, p.agent_id, p.joined_at,
       u.display_name AS user_display_name,
       a.display_name AS agent_display_name,
       a.handle       AS agent_handle,
       -- Whether the agent has a published picture, so the app knows to
       -- ask for it rather than guessing and getting a 404.
       (a.avatar_media_id IS NOT NULL)::bool AS agent_has_avatar,
       -- What the agent suggests saying first, for an empty chat.
       a.starters     AS agent_starters,
       -- The dot on the avatar: the agent's live binding's health, or
       -- nothing when no backend is connected.
       b.status       AS agent_status
FROM participants p
LEFT JOIN users  u ON u.id = p.user_id
LEFT JOIN agents a ON a.id = p.agent_id
LEFT JOIN agent_bindings b ON b.agent_id = p.agent_id AND b.revoked_at IS NULL
WHERE p.conversation_id = ANY(sqlc.arg('conversation_ids')::uuid[])
ORDER BY p.conversation_id, p.joined_at, p.kind, p.user_id, p.agent_id;

-- name: ListAgentParticipants :many
-- The agents in a conversation: who must hear a message posted in it.
SELECT agent_id::uuid AS agent_id
FROM participants
WHERE conversation_id = $1 AND agent_id IS NOT NULL
ORDER BY joined_at, agent_id;

-- name: ListConversationsByIDs :many
SELECT * FROM conversations
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListUsersSharingAgent :many
-- Everyone who has a conversation with an agent: who is told, live, when
-- the agent's backend comes and goes. Today that is its owner; once agents
-- are handed out it is everyone who added one.
SELECT DISTINCT p.user_id::uuid AS user_id
FROM participants ap
JOIN participants p ON p.conversation_id = ap.conversation_id AND p.user_id IS NOT NULL
WHERE ap.agent_id = $1::uuid;

-- name: ListUserParticipants :many
-- The people in a conversation: who is told, live, when something happens
-- in it.
SELECT user_id::uuid AS user_id
FROM participants
WHERE conversation_id = $1 AND user_id IS NOT NULL
ORDER BY joined_at, user_id;
