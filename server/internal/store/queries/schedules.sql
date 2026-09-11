-- name: CreateSchedule :one
INSERT INTO schedules (id, agent_id, conversation_id, title, instruction, cadence, status, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSchedule :one
-- Deleted ones too: a refusal has to be able to say "deleted".
SELECT * FROM schedules
WHERE id = $1;

-- name: ListSchedules :many
SELECT * FROM schedules
WHERE conversation_id = $1 AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: CountSchedules :one
SELECT count(*)::int FROM schedules
WHERE conversation_id = $1 AND deleted_at IS NULL;

-- name: UpdateSchedule :one
-- Partial: a null argument leaves that column as it is. Never a deleted one.
UPDATE schedules
SET title       = COALESCE(sqlc.narg('title')::text, title),
    instruction = COALESCE(sqlc.narg('instruction')::text, instruction),
    cadence     = COALESCE(sqlc.narg('cadence')::jsonb, cadence),
    status      = COALESCE(sqlc.narg('status')::text, status),
    updated_at  = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: DeleteSchedule :one
UPDATE schedules
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ListPersonSchedulesInZone :many
-- A person's live schedules set to one time zone: the ones that move with
-- them when their phone's clock does.
SELECT s.* FROM schedules s
JOIN participants p ON p.conversation_id = s.conversation_id AND p.user_id = sqlc.arg('user_id')::uuid
WHERE s.deleted_at IS NULL AND s.cadence->>'timezone' = sqlc.arg('timezone')::text
ORDER BY s.created_at;

-- name: MarkScheduleRan :exec
UPDATE schedules SET last_run_at = now()
WHERE id = $1;

-- name: ListScheduleTitles :many
-- The names a batch of messages was sent under, for the tag on the bubble.
SELECT id, title FROM schedules
WHERE id = ANY(sqlc.arg('ids')::uuid[]);
