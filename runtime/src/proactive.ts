/**
 * The budget for speaking first.
 *
 * The hub allows an agent to send into a conversation nobody is looking at,
 * and counts nothing. So the count lives here.
 *
 * Ten a day per person (Q12) is not bureaucracy. A bug in a scheduler is
 * somebody's phone at three in the morning, and the only remedy they have is
 * blocking the agent — which they will do once, and for good. The limit is
 * what makes a scheduler bug embarrassing rather than final.
 */

import type { Pool } from "./db/pool.ts";

const DEFAULT_DAILY = 10;

export class Proactive {
  private readonly pool: Pool;
  private readonly daily: number;

  constructor(pool: Pool, daily = DEFAULT_DAILY) {
    this.pool = pool;
    this.daily = daily;
  }

  /** How many unprompted messages this person has had in the last day. */
  async sentToday(userId: string): Promise<number> {
    const { rows } = await this.pool.query<{ n: string }>(
      `SELECT count(*) AS n FROM proactive_sends
       WHERE user_id = $1 AND sent_at > now() - interval '24 hours'`,
      [userId],
    );
    return Number(rows[0]?.n ?? 0);
  }

  /** Whether another one may go out. Asked before sending, never after. */
  async maySend(userId: string): Promise<boolean> {
    if (this.daily <= 0) return false;
    return (await this.sentToday(userId)) < this.daily;
  }

  async record(input: {
    userId: string;
    agentId: string;
    conversationId: string;
    reason: string;
  }): Promise<void> {
    await this.pool.query(
      `INSERT INTO proactive_sends (user_id, agent_id, conversation_id, reason)
       VALUES ($1, $2, $3, $4)`,
      [input.userId, input.agentId, input.conversationId, input.reason],
    );
  }
}
