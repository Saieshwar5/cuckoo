-- name: ListExpiredMessageIDs :many
-- The oldest messages past the window, a batch at a time. A stream still
-- being written is never old enough to be here, and is left alone anyway.
SELECT id FROM messages
WHERE created_at < sqlc.arg('before') AND status = 'complete'
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: CountExpiredMessages :one
SELECT count(*) FROM messages
WHERE created_at < sqlc.arg('before') AND status = 'complete';

-- name: DeleteMediaOfMessages :many
-- The files hanging on a batch of expiring messages. Their keys come back so
-- the bytes go too.
DELETE FROM media
WHERE message_id = ANY(sqlc.arg('message_ids')::uuid[])
RETURNING storage_key, thumb_key;

-- name: DeleteMessagesByIDs :execrows
-- Delivery rows go with them (ON DELETE CASCADE); everything else that
-- named a message keeps its id.
DELETE FROM messages WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: DeleteFinishedDeliveries :execrows
-- Delivery rows are proof that an event reached a backend, not history.
-- Once that proof is old enough it has served its purpose.
DELETE FROM message_deliveries
WHERE status <> 'pending'
  AND COALESCE(delivered_at, created_at) < sqlc.arg('before')::timestamptz;

-- name: ListMediaOwnersOverBudget :many
-- Whose sent files add up to more than they are allowed to keep on the hub.
-- Profile pictures hang on a profile rather than a message and are not
-- counted, so a face is never what gets removed.
SELECT owner_id, SUM(byte_size)::bigint AS total_bytes
FROM media
WHERE owner_kind = sqlc.arg('owner_kind') AND message_id IS NOT NULL
GROUP BY owner_id
HAVING SUM(byte_size) > sqlc.arg('budget');

-- name: ListClaimedMediaOldestFirst :many
-- One owner's sent files, oldest first: the order they go in when they are
-- over budget.
SELECT id, byte_size, storage_key, thumb_key
FROM media
WHERE owner_kind = sqlc.arg('owner_kind')
  AND owner_id = sqlc.arg('owner_id')
  AND message_id IS NOT NULL
ORDER BY created_at, id
LIMIT sqlc.arg('batch');

-- name: DeleteMediaByIDs :many
DELETE FROM media
WHERE id = ANY(sqlc.arg('ids')::uuid[])
RETURNING storage_key, thumb_key;

-- name: SumClaimedMediaByOwner :one
-- How much of their budget one owner is using.
SELECT COALESCE(SUM(byte_size), 0)::bigint
FROM media
WHERE owner_kind = sqlc.arg('owner_kind')
  AND owner_id = sqlc.arg('owner_id')
  AND message_id IS NOT NULL;
