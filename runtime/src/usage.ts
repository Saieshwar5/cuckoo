/**
 * What people spend, and the ceiling that stops one conversation spending all
 * night.
 *
 * Cuckoo pays for the model on agents it runs (D68), so this is a real bill
 * rather than a statistic. A trial and then a subscription will read the same
 * rows; until there is a subscription, the ceiling is the whole product of it.
 *
 * The limit is per person per day and deliberately blunt. A refusal that is
 * explained is a bad afternoon; an unbounded bill is a bad month.
 */

import type { Pool } from "./db/pool.ts";

export interface Spend {
  inputTokens: number;
  outputTokens: number;
  runs: number;
}

export class Usage {
  private readonly pool: Pool;
  private readonly dailyLimit: number;

  /** `dailyLimit` of 0 means no ceiling — for development, not for a server. */
  constructor(pool: Pool, dailyLimit: number) {
    this.pool = pool;
    this.dailyLimit = dailyLimit;
  }

  /** What this person has spent today, across every agent of theirs. */
  async spentToday(userId: string): Promise<Spend> {
    const { rows } = await this.pool.query<{
      input: string | null;
      output: string | null;
      runs: string | null;
    }>(
      `SELECT sum(input_tokens) AS input, sum(output_tokens) AS output, sum(runs) AS runs
       FROM usage WHERE user_id = $1 AND day = current_date`,
      [userId],
    );
    const row = rows[0];
    return {
      inputTokens: Number(row?.input ?? 0),
      outputTokens: Number(row?.output ?? 0),
      runs: Number(row?.runs ?? 0),
    };
  }

  /**
   * Whether this person may be answered at all right now.
   *
   * Checked before a run rather than during: a run that is refused costs
   * nothing, and a run cut off halfway has already been paid for and leaves a
   * half-written reply on somebody's screen.
   */
  async withinLimit(userId: string): Promise<boolean> {
    if (this.dailyLimit <= 0) return true;
    const spent = await this.spentToday(userId);
    return spent.inputTokens + spent.outputTokens < this.dailyLimit;
  }

  /** Add what a run cost. Called after it, whatever the run's outcome. */
  async record(
    userId: string,
    agentId: string,
    tokens: { inputTokens: number; outputTokens: number },
  ): Promise<void> {
    await this.pool.query(
      `INSERT INTO usage (user_id, agent_id, day, input_tokens, output_tokens, runs)
       VALUES ($1, $2, current_date, $3, $4, 1)
       ON CONFLICT (user_id, agent_id, day) DO UPDATE
         SET input_tokens  = usage.input_tokens  + EXCLUDED.input_tokens,
             output_tokens = usage.output_tokens + EXCLUDED.output_tokens,
             runs          = usage.runs + 1,
             updated_at    = now()`,
      [userId, agentId, tokens.inputTokens, tokens.outputTokens],
    );
  }
}
