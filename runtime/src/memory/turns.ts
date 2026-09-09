/**
 * What was said, kept in full — and the part of it the model is shown.
 *
 * These are two different things, and conflating them is how an agent ends up
 * with the memory of a goldfish. Everything is written down here forever; a
 * run is given the persona, the recent turns, and (when compaction arrives) a
 * summary of what came before. The model's context window is the model's
 * limit, not the product's.
 */

import type { Pool } from "../db/pool.ts";

export type Role = "user" | "assistant" | "tool";

export interface Turn {
  id: string;
  role: Role;
  text: string;
  toolName?: string;
  toolArgs?: unknown;
  toolResult?: unknown;
  hubMessageId?: string;
  createdAt: string;
}

export interface RecordInput {
  agentId: string;
  conversationId: string;
  role: Role;
  text?: string;
  toolName?: string;
  toolArgs?: unknown;
  toolResult?: unknown;
  hubMessageId?: string;
}

/** How many turns a run is shown, until compaction decides better. */
export const DEFAULT_WINDOW = 40;

/** What `answer` needs of the store, so a test can stand one in. */
export interface TurnStore {
  record(input: RecordInput): Promise<void>;
  window(agentId: string, conversationId: string, limit?: number): Promise<Turn[]>;
}

export class Turns implements TurnStore {
  private readonly pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  async record(input: RecordInput): Promise<void> {
    await this.pool.query(
      `INSERT INTO turns (agent_id, conversation_id, role, text, tool_name, tool_args,
                          tool_result, hub_message_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
      [
        input.agentId,
        input.conversationId,
        input.role,
        input.text ?? "",
        input.toolName ?? null,
        input.toolArgs === undefined ? null : JSON.stringify(input.toolArgs),
        input.toolResult === undefined ? null : JSON.stringify(input.toolResult),
        input.hubMessageId ?? null,
      ],
    );
  }

  /**
   * The window a run is given: the most recent turns, oldest first.
   *
   * Read newest-first and reversed, because "the last forty" is an index seek
   * and "the first forty of everything" is a table scan that gets slower every
   * day the person uses their agent.
   */
  async window(
    agentId: string,
    conversationId: string,
    limit = DEFAULT_WINDOW,
  ): Promise<Turn[]> {
    const { rows } = await this.pool.query<{
      id: string;
      role: Role;
      text: string;
      tool_name: string | null;
      tool_args: unknown;
      tool_result: unknown;
      hub_message_id: string | null;
      created_at: Date;
    }>(
      `SELECT id, role, text, tool_name, tool_args, tool_result, hub_message_id, created_at
       FROM turns
       WHERE agent_id = $1 AND conversation_id = $2
       ORDER BY id DESC
       LIMIT $3`,
      [agentId, conversationId, limit],
    );

    return rows
      .map((row): Turn => ({
        id: String(row.id),
        role: row.role,
        text: row.text,
        ...(row.tool_name ? { toolName: row.tool_name } : {}),
        ...(row.tool_args !== null ? { toolArgs: row.tool_args } : {}),
        ...(row.tool_result !== null ? { toolResult: row.tool_result } : {}),
        ...(row.hub_message_id ? { hubMessageId: row.hub_message_id } : {}),
        createdAt: row.created_at.toISOString(),
      }))
      .reverse();
  }

  /** How much is remembered here. What compaction will one day act on. */
  async count(agentId: string, conversationId: string): Promise<number> {
    const { rows } = await this.pool.query<{ n: string }>(
      "SELECT count(*) AS n FROM turns WHERE agent_id = $1 AND conversation_id = $2",
      [agentId, conversationId],
    );
    return Number(rows[0]?.n ?? 0);
  }
}
