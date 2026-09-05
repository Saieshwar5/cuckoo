-- name: CreateDelivery :one
-- A message event names its message; a membership event carries a payload.
INSERT INTO message_deliveries (id, message_id, conversation_id, agent_id, event_type, payload, status, last_error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetDelivery :one
SELECT * FROM message_deliveries
WHERE id = $1;

-- name: ListDeliveriesByMessage :many
SELECT * FROM message_deliveries
WHERE message_id = sqlc.arg('message_id')::uuid
ORDER BY id;

-- name: ClaimDueWebhookDeliveries :many
-- Leases a batch of due deliveries to one worker.
--
-- SKIP LOCKED means two workers never take the same row. The lease is the
-- bumped next_attempt_at: if the process dies mid-delivery the row simply
-- comes due again, so delivery is at-least-once without any in-flight state
-- to clean up. Only agents whose live binding is a webhook are eligible;
-- socket-bound deliveries wait for the socket transport.
UPDATE message_deliveries d
SET attempts        = d.attempts + 1,
    next_attempt_at = now() + interval '30 seconds'
WHERE d.id IN (
    SELECT due.id
    FROM message_deliveries due
    WHERE due.status = 'pending'
      AND due.next_attempt_at <= now()
      AND EXISTS (
          SELECT 1 FROM agent_bindings b
          WHERE b.agent_id = due.agent_id
            AND b.revoked_at IS NULL
            AND b.mode = 'webhook'
      )
    ORDER BY due.next_attempt_at
    LIMIT sqlc.arg('batch_size')
    FOR UPDATE OF due SKIP LOCKED
)
RETURNING d.*;

-- name: ClaimDueSocketDeliveries :many
-- Leases one agent's due deliveries to the socket holding it, oldest first,
-- with the same thirty-second lease the webhook worker uses: a pushed event
-- that is not acknowledged in time comes due again and is pushed again.
UPDATE message_deliveries d
SET attempts        = d.attempts + 1,
    next_attempt_at = now() + interval '30 seconds'
WHERE d.id IN (
    SELECT due.id
    FROM message_deliveries due
    WHERE due.agent_id = sqlc.arg('agent_id')
      AND due.status = 'pending'
      AND due.next_attempt_at <= now()
      AND EXISTS (
          SELECT 1 FROM agent_bindings b
          WHERE b.agent_id = due.agent_id
            AND b.revoked_at IS NULL
            AND b.mode = 'socket'
      )
    ORDER BY due.id
    LIMIT sqlc.arg('batch_size')
    FOR UPDATE OF due SKIP LOCKED
)
RETURNING d.*;

-- name: AckDelivery :one
-- A backend acknowledging an event over its socket. Scoped to the agent so
-- a backend can only ever acknowledge its own.
UPDATE message_deliveries
SET status = 'delivered', delivered_at = now(), last_error = NULL
WHERE id = sqlc.arg('id') AND agent_id = sqlc.arg('agent_id') AND status = 'pending'
RETURNING message_id;

-- name: MarkDelivered :exec
UPDATE message_deliveries
SET status = 'delivered', delivered_at = now(), last_error = NULL
WHERE id = $1 AND status = 'pending';

-- name: ScheduleRetry :exec
UPDATE message_deliveries
SET next_attempt_at = sqlc.arg('next_attempt_at'), last_error = sqlc.arg('last_error')
WHERE id = sqlc.arg('id') AND status = 'pending';

-- name: MarkFailed :exec
UPDATE message_deliveries
SET status = 'failed', last_error = sqlc.arg('last_error')
WHERE id = sqlc.arg('id') AND status = 'pending';

-- name: FailExpiredDeliveries :execrows
-- Anything still pending a day after it was created has run out of retries,
-- whatever transport it was waiting for.
UPDATE message_deliveries
SET status = 'failed', last_error = 'expired'
WHERE status = 'pending' AND created_at < now() - interval '24 hours';

-- name: FailUnboundDeliveries :execrows
-- A pending delivery whose agent no longer has any live binding was sent
-- before the binding was revoked. Nothing will ever attempt it, and the user
-- should see "not delivered" now rather than after a day.
UPDATE message_deliveries d
SET status = 'failed', last_error = 'no_binding'
WHERE d.status = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM agent_bindings b
      WHERE b.agent_id = d.agent_id AND b.revoked_at IS NULL
  );

-- name: ListDeliveriesSince :many
-- An agent's events in order, for a backend catching up after downtime.
SELECT * FROM message_deliveries
WHERE agent_id = sqlc.arg('agent_id')
  AND (sqlc.narg('since')::uuid IS NULL OR id > sqlc.narg('since')::uuid)
ORDER BY id
LIMIT sqlc.arg('page_size');

-- name: SummarizeDeliveries :many
-- Per message: how many backends it was for, and how many have it.
SELECT message_id::uuid AS message_id,
       count(*)::int                                     AS total,
       count(*) FILTER (WHERE status = 'delivered')::int AS delivered,
       count(*) FILTER (WHERE status = 'failed')::int    AS failed
FROM message_deliveries
WHERE message_id = ANY(sqlc.arg('message_ids')::uuid[])
GROUP BY message_id;
