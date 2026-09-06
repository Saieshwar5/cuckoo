-- name: CreateMedia :one
INSERT INTO media (id, owner_kind, owner_id, kind, mime_type, byte_size, file_name,
                   width, height, storage_key, thumb_key, duration_ms, waveform)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1;

-- name: ListMediaByIDs :many
-- The files a send is claiming, read in one query so the order the caller
-- listed them in can be preserved by the caller.
SELECT * FROM media WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ClaimMedia :execrows
-- Hang uploaded files on the message that carries them.
--
-- The row count is the check: claiming refuses a file that is not the
-- sender's, or that some earlier message already carries, and the send sees
-- fewer rows than it asked for and fails.
UPDATE media
   SET message_id = sqlc.arg('message_id')
 WHERE id = ANY(sqlc.arg('ids')::uuid[])
   AND message_id IS NULL
   AND owner_kind = sqlc.arg('owner_kind')
   AND owner_id = sqlc.arg('owner_id')::uuid;

-- name: UserCanReadMedia :one
-- A person may read a file they uploaded, or one hanging on a message in a
-- conversation they are in.
SELECT EXISTS (
    SELECT 1 FROM media m
    WHERE m.id = sqlc.arg('id')
      AND (
          (m.owner_kind = 'user' AND m.owner_id = sqlc.arg('user_id')::uuid)
          OR EXISTS (
              SELECT 1 FROM messages msg
              JOIN participants p ON p.conversation_id = msg.conversation_id
              WHERE msg.id = m.message_id AND p.user_id = sqlc.arg('user_id')::uuid
          )
      )
);

-- name: AgentCanReadMedia :one
-- The same rule for a backend: its own uploads, or files on messages in
-- conversations it is a member of.
SELECT EXISTS (
    SELECT 1 FROM media m
    WHERE m.id = sqlc.arg('id')
      AND (
          (m.owner_kind = 'agent' AND m.owner_id = sqlc.arg('agent_id')::uuid)
          OR EXISTS (
              SELECT 1 FROM messages msg
              JOIN participants p ON p.conversation_id = msg.conversation_id
              WHERE msg.id = m.message_id AND p.agent_id = sqlc.arg('agent_id')::uuid
          )
      )
);

-- name: GetAgentAvatar :one
-- The picture an agent is published with. Public: this is what a stranger
-- deciding whether to add it looks at.
SELECT m.* FROM media m
  JOIN agents a ON a.avatar_media_id = m.id
 WHERE a.id = sqlc.arg('agent_id') AND a.deleted_at IS NULL;

-- name: DeleteUnclaimedMedia :many
-- Uploads nobody ever sent, so an abandoned pick does not sit on the disk
-- forever. Returns their storage keys so the bytes go too.
--
-- A face is claimed by a profile rather than by a message, so the two
-- profiles that can point at one are asked before anything is removed.
DELETE FROM media
 WHERE media.message_id IS NULL
   AND media.created_at < sqlc.arg('before')
   AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.avatar_media_id = media.id)
   AND NOT EXISTS (SELECT 1 FROM users u WHERE u.avatar_media_id = media.id)
RETURNING storage_key, thumb_key;
