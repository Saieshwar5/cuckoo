/**
 * Confirmations: what a button will do when it is tapped.
 *
 * Nothing that changes the world happens because a model said so. The agent
 * offers, the person taps, and the tap is looked up here and carried out by
 * this code. The row is what was agreed to — which is the audit trail, and
 * which is why the offer can be trusted even when the model that wrote it
 * cannot be.
 *
 * The model chooses the words. It does not choose whether there is a button,
 * and it cannot act by claiming one was tapped.
 */

import { randomUUID } from "node:crypto";

import type { Pool } from "./db/pool.ts";

/** What a tap may set in motion. Named, so a tap can only run what we know. */
export type ActionName = "create_routine" | "delete_routine";

export interface PendingAction {
  buttonId: string;
  agentId: string;
  conversationId: string;
  userId: string;
  action: ActionName;
  arguments: Record<string, unknown>;
}

/** How long an offer stands. Long enough to think, short enough to forget. */
const TTL_MINUTES = 60;

export class Actions {
  private readonly pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  /** Record what a button would do, and return its id for the agent to offer. */
  async offer(input: {
    agentId: string;
    conversationId: string;
    userId: string;
    action: ActionName;
    arguments: Record<string, unknown>;
  }): Promise<string> {
    const buttonId = `act_${randomUUID().replace(/-/g, "").slice(0, 20)}`;
    await this.pool.query(
      `INSERT INTO pending_actions (button_id, agent_id, conversation_id, user_id, action,
                                    arguments, expires_at)
       VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(mins => $7))`,
      [
        buttonId,
        input.agentId,
        input.conversationId,
        input.userId,
        input.action,
        JSON.stringify(input.arguments),
        TTL_MINUTES,
      ],
    );
    return buttonId;
  }

  /**
   * Claim a tapped button, once.
   *
   * The claim is the same statement as the read, so two taps — a double press,
   * a retried delivery — cannot both find it waiting. Scoped to the agent and
   * the conversation, so a button id learned from somewhere else is not enough.
   */
  async take(
    buttonId: string,
    agentId: string,
    conversationId: string,
  ): Promise<PendingAction | undefined> {
    const { rows } = await this.pool.query<{
      button_id: string;
      agent_id: string;
      conversation_id: string;
      user_id: string;
      action: ActionName;
      arguments: Record<string, unknown>;
    }>(
      `UPDATE pending_actions SET taken_at = now()
       WHERE button_id = $1 AND agent_id = $2 AND conversation_id = $3
         AND taken_at IS NULL AND expires_at > now()
       RETURNING button_id, agent_id, conversation_id, user_id, action, arguments`,
      [buttonId, agentId, conversationId],
    );
    const row = rows[0];
    if (!row) return undefined;
    return {
      buttonId: row.button_id,
      agentId: row.agent_id,
      conversationId: row.conversation_id,
      userId: row.user_id,
      action: row.action,
      arguments: row.arguments ?? {},
    };
  }
}
