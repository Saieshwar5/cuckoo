/**
 * Standing instructions, and when they next run.
 *
 * A routine is not a second kind of conversation. It is the same conversation
 * on a timer: the person's own words, run as though they had just said them.
 * That is why there is so little here — a sentence and a schedule.
 */

import { CronExpressionParser } from "cron-parser";
import { randomUUID } from "node:crypto";

import type { Pool } from "./db/pool.ts";

export interface Routine {
  id: string;
  agentId: string;
  conversationId: string;
  userId: string;
  instruction: string;
  schedule: string;
  timezone: string;
  nextRun: string;
  paused: boolean;
  /** Its twin on the hub, the schedule the person sees; null until mirrored. */
  hubScheduleId: string | null;
  title: string;
  /** Runs once and goes. */
  once: boolean;
}

/** A routine that fails this many times running is paused and says so. */
const FAILURES_BEFORE_PAUSE = 3;

/**
 * When a schedule next comes round, in its own zone.
 *
 * "Eight in the morning" is a claim about where somebody is standing, so the
 * zone is part of the schedule rather than a detail of how it is stored.
 * Throws on a schedule that is not one, which is what validates the model's
 * suggestion before a person is asked to agree to it.
 */
export function nextRun(schedule: string, timezone: string, after = new Date()): Date {
  const parsed = CronExpressionParser.parse(schedule, { tz: timezone, currentDate: after });
  return parsed.next().toDate();
}

/** Whether a schedule and zone are ones this can act on. */
export function validSchedule(schedule: string, timezone: string): boolean {
  try {
    nextRun(schedule, timezone);
    return true;
  } catch {
    return false;
  }
}

export class Routines {
  private readonly pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  async create(input: {
    agentId: string;
    conversationId: string;
    userId: string;
    instruction: string;
    schedule: string;
    timezone?: string;
    title?: string;
    once?: boolean;
    hubScheduleId?: string;
  }): Promise<Routine> {
    const timezone = input.timezone || "Asia/Kolkata";
    const first = nextRun(input.schedule, timezone);
    const { rows } = await this.pool.query<Row>(
      `INSERT INTO routines (id, agent_id, conversation_id, user_id, instruction, schedule,
                             timezone, next_run, title, once, hub_schedule_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
       RETURNING *`,
      [
        randomUUID(),
        input.agentId,
        input.conversationId,
        input.userId,
        input.instruction,
        input.schedule,
        timezone,
        first,
        input.title ?? "",
        input.once ?? false,
        input.hubScheduleId ?? null,
      ],
    );
    return toRoutine(rows[0]!);
  }

  async findByHubId(hubScheduleId: string): Promise<Routine | undefined> {
    const { rows } = await this.pool.query<Row>("SELECT * FROM routines WHERE hub_schedule_id = $1", [
      hubScheduleId,
    ]);
    return rows[0] ? toRoutine(rows[0]) : undefined;
  }

  async setHubId(id: string, hubScheduleId: string): Promise<void> {
    await this.pool.query("UPDATE routines SET hub_schedule_id = $2 WHERE id = $1", [id, hubScheduleId]);
  }

  /** Routines the hub has never heard of: made before the app could see them. */
  async unmirrored(limit = 50): Promise<Routine[]> {
    const { rows } = await this.pool.query<Row>(
      "SELECT * FROM routines WHERE hub_schedule_id IS NULL ORDER BY created_at LIMIT $1",
      [limit],
    );
    return rows.map(toRoutine);
  }

  /** A new what or when, from the person, and the next run worked out again. */
  async change(
    id: string,
    input: { instruction: string; schedule: string; timezone: string; once: boolean; title: string },
  ): Promise<void> {
    await this.pool.query(
      `UPDATE routines SET instruction = $2, schedule = $3, timezone = $4, once = $5, title = $6,
              next_run = $7, failures = 0, paused_at = NULL, paused_reason = NULL
       WHERE id = $1`,
      [id, input.instruction, input.schedule, input.timezone, input.once, input.title,
        nextRun(input.schedule, input.timezone)],
    );
  }

  /** Running again, from its next time — not from every time it missed. */
  async resume(id: string): Promise<void> {
    const { rows } = await this.pool.query<Row>("SELECT * FROM routines WHERE id = $1", [id]);
    const routine = rows[0];
    if (!routine) return;
    await this.pool.query(
      "UPDATE routines SET paused_at = NULL, paused_reason = NULL, failures = 0, next_run = $2 WHERE id = $1",
      [id, nextRun(routine.schedule, routine.timezone)],
    );
  }

  /**
   * Take up to `limit` routines that are due.
   *
   * The claim pushes `next_run` an hour out before anything is run, which does
   * two jobs: `SKIP LOCKED` keeps a second process off the same row, and the
   * hour keeps a slow run from being claimed again by the next pass. The real
   * next time is computed straight afterwards, because cron is parsed here and
   * not in the database. If this process dies in the gap, the routine runs an
   * hour late rather than twice or never — the failure worth having.
   */
  async claimDue(limit = 10): Promise<Routine[]> {
    const { rows } = await this.pool.query<Row>(
      `UPDATE routines SET last_run = now(), next_run = now() + interval '1 hour'
       WHERE id IN (
         SELECT id FROM routines
         WHERE paused_at IS NULL AND next_run <= now()
         ORDER BY next_run
         FOR UPDATE SKIP LOCKED
         LIMIT $1
       )
       RETURNING *`,
      [limit],
    );

    const claimed = rows.map(toRoutine);
    for (const routine of claimed) {
      // A one-off keeps its hour's grace: it is deleted once it has run, and
      // a run that failed is tried again then.
      if (routine.once) continue;
      // A schedule that no longer parses would spin forever an hour at a
      // time; pausing says so once instead.
      try {
        await this.reschedule(routine.id, routine.schedule, routine.timezone);
      } catch {
        await this.pause(routine.id, "its schedule could not be read");
      }
    }
    return claimed;
  }

  async pause(id: string, reason: string): Promise<void> {
    await this.pool.query(
      "UPDATE routines SET paused_at = now(), paused_reason = $2 WHERE id = $1",
      [id, reason],
    );
  }

  /** Its next time, computed here rather than in the database. */
  async reschedule(id: string, schedule: string, timezone: string): Promise<void> {
    await this.pool.query("UPDATE routines SET next_run = $2, failures = 0 WHERE id = $1", [
      id,
      nextRun(schedule, timezone),
    ]);
  }

  /**
   * Record that a run failed. Past a few in a row the routine is paused, and
   * the caller is told to say so — a stopped routine and a silent one look the
   * same to the person unless somebody tells them.
   */
  async recordFailure(id: string, reason: string): Promise<{ paused: boolean }> {
    const { rows } = await this.pool.query<{ failures: number; paused_at: Date | null }>(
      `UPDATE routines
       SET failures = failures + 1,
           paused_at = CASE WHEN failures + 1 >= $2 THEN now() ELSE paused_at END,
           paused_reason = CASE WHEN failures + 1 >= $2 THEN $3 ELSE paused_reason END
       WHERE id = $1
       RETURNING failures, paused_at`,
      [id, FAILURES_BEFORE_PAUSE, reason],
    );
    return { paused: rows[0]?.paused_at != null };
  }

  async listFor(agentId: string, conversationId: string): Promise<Routine[]> {
    const { rows } = await this.pool.query<Row>(
      `SELECT * FROM routines WHERE agent_id = $1 AND conversation_id = $2
       ORDER BY created_at`,
      [agentId, conversationId],
    );
    return rows.map(toRoutine);
  }

  async delete(id: string, agentId: string): Promise<boolean> {
    const { rowCount } = await this.pool.query(
      "DELETE FROM routines WHERE id = $1 AND agent_id = $2",
      [id, agentId],
    );
    return (rowCount ?? 0) > 0;
  }
}

interface Row {
  id: string;
  agent_id: string;
  conversation_id: string;
  user_id: string;
  instruction: string;
  schedule: string;
  timezone: string;
  next_run: Date;
  paused_at: Date | null;
  hub_schedule_id: string | null;
  title: string;
  once: boolean;
}

function toRoutine(row: Row): Routine {
  return {
    id: row.id,
    agentId: row.agent_id,
    conversationId: row.conversation_id,
    userId: row.user_id,
    instruction: row.instruction,
    schedule: row.schedule,
    timezone: row.timezone,
    nextRun: row.next_run.toISOString(),
    paused: row.paused_at != null,
    hubScheduleId: row.hub_schedule_id,
    title: row.title,
    once: row.once,
  };
}
