/**
 * Work written down before it is done, and the worker that does it.
 *
 * The hub gives a webhook ten seconds and retries anything slower. A model
 * takes longer than that to think. If the two were the same step, every slow
 * answer would arrive twice — so the receiver writes a row and answers, and
 * this picks it up afterwards.
 *
 * De-duplication is the unique index on event_id: the hub redelivers what it
 * never saw acknowledged, and the second insert does nothing.
 */

import type { Pool } from "./db/pool.ts";

export interface Job {
  id: string;
  agentId: string;
  eventId: string;
  type: string;
  payload: Record<string, unknown>;
  attempts: number;
}

/** How long to wait before trying a failed job again, by attempt. */
const BACKOFF_SECONDS = [5, 30, 120, 600];
const MAX_ATTEMPTS = 5;

export class Jobs {
  private readonly pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  /** Write a job down. False means this event was already known. */
  async enqueue(input: {
    agentId: string;
    eventId: string;
    type: string;
    payload: Record<string, unknown>;
  }): Promise<boolean> {
    const { rowCount } = await this.pool.query(
      `INSERT INTO jobs (agent_id, event_id, type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (event_id) DO NOTHING`,
      [input.agentId, input.eventId, input.type, JSON.stringify(input.payload)],
    );
    return (rowCount ?? 0) > 0;
  }

  /**
   * Take up to `limit` due jobs, marking them running in the same statement
   * so two workers never take the same one. `SKIP LOCKED` is what makes a
   * second process a second worker rather than a queue behind the first.
   */
  async claim(limit = 5): Promise<Job[]> {
    const { rows } = await this.pool.query<{
      id: string;
      agent_id: string;
      event_id: string;
      type: string;
      payload: Record<string, unknown>;
      attempts: number;
    }>(
      `UPDATE jobs SET status = 'running', claimed_at = now(), attempts = attempts + 1
       WHERE id IN (
         SELECT id FROM jobs
         WHERE status = 'pending' AND run_at <= now()
         ORDER BY run_at
         FOR UPDATE SKIP LOCKED
         LIMIT $1
       )
       RETURNING id, agent_id, event_id, type, payload, attempts`,
      [limit],
    );
    return rows.map((row) => ({
      id: String(row.id),
      agentId: row.agent_id,
      eventId: row.event_id,
      type: row.type,
      payload: row.payload,
      attempts: row.attempts,
    }));
  }

  async complete(id: string): Promise<void> {
    await this.pool.query("UPDATE jobs SET status = 'done', last_error = NULL WHERE id = $1", [id]);
  }

  /**
   * Record a failure and decide whether to try again. Past the last attempt
   * the job stays failed: something that has broken five times is not going
   * to work on the sixth, and a person waiting has long since given up.
   */
  async fail(job: Job, error: unknown): Promise<void> {
    const message = error instanceof Error ? error.message : String(error);
    if (job.attempts >= MAX_ATTEMPTS) {
      await this.pool.query("UPDATE jobs SET status = 'failed', last_error = $2 WHERE id = $1", [
        job.id,
        message,
      ]);
      return;
    }
    const wait = BACKOFF_SECONDS[job.attempts - 1] ?? BACKOFF_SECONDS.at(-1) ?? 600;
    await this.pool.query(
      `UPDATE jobs SET status = 'pending', last_error = $2,
              run_at = now() + make_interval(secs => $3)
       WHERE id = $1`,
      [job.id, message, wait],
    );
  }

  /** Anything left running when the process died is due again. */
  async recoverAbandoned(olderThanSeconds = 300): Promise<number> {
    const { rowCount } = await this.pool.query(
      `UPDATE jobs SET status = 'pending'
       WHERE status = 'running' AND claimed_at < now() - make_interval(secs => $1)`,
      [olderThanSeconds],
    );
    return rowCount ?? 0;
  }
}

export type JobHandler = (job: Job) => Promise<void>;

/**
 * Polls for work. A poll rather than a listen because Postgres is already
 * here and a second piece of infrastructure to hold a queue is a second thing
 * to run, watch and restart.
 */
export class Worker {
  private readonly jobs: Jobs;
  private readonly handle: JobHandler;
  private readonly intervalMs: number;
  private running = false;
  private timer?: ReturnType<typeof setTimeout>;

  constructor(jobs: Jobs, handle: JobHandler, options: { intervalMs?: number } = {}) {
    this.jobs = jobs;
    this.handle = handle;
    this.intervalMs = options.intervalMs ?? 500;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    void this.loop();
  }

  async stop(): Promise<void> {
    this.running = false;
    if (this.timer) clearTimeout(this.timer);
  }

  /** One pass, for tests and for the loop. Returns how many were done. */
  async tick(): Promise<number> {
    const claimed = await this.jobs.claim();
    for (const job of claimed) {
      try {
        await this.handle(job);
        await this.jobs.complete(job.id);
      } catch (error) {
        console.error(`runtime: job ${job.id} (${job.type}) failed`, error);
        await this.jobs.fail(job, error);
      }
    }
    return claimed.length;
  }

  private async loop(): Promise<void> {
    while (this.running) {
      let did = 0;
      try {
        did = await this.tick();
      } catch (error) {
        // The database is unreachable, or something else the loop itself
        // cannot fix. Wait and try again rather than exiting: a runtime that
        // dies on a blip needs a person to notice.
        console.error("runtime: worker pass failed", error);
      }
      // Straight back round when there was work: a busy queue should not be
      // paced by the poll interval.
      if (!did) await new Promise((resolve) => { this.timer = setTimeout(resolve, this.intervalMs); });
    }
  }
}
