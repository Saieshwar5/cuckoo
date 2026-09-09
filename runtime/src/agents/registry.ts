/**
 * Which agents this runtime answers for, and what each one is.
 *
 * One process serves every agent: a person's three, and the ready-made ones
 * ten thousand people share. An agent is a row, not a running thing — nothing
 * is started or held open per agent, which is what makes "one backend for
 * everyone" a sentence about resources and not just about architecture.
 */

import { randomUUID } from "node:crypto";

import { decrypt, encrypt } from "../crypto.ts";
import type { Pool } from "../db/pool.ts";

export interface Template {
  id: string;
  name: string;
  persona: string;
  tools: string[];
  model: string;
  starters: string[];
}

export interface AgentRecord {
  id: string;
  hubAgentId: string;
  ownerUserId: string;
  template: Template;
  /** The person's own words, when they wrote some; the template's otherwise. */
  persona: string;
  model: string;
  private: boolean;
}

interface Row {
  id: string;
  hub_agent_id: string;
  owner_user_id: string;
  persona: string;
  model: string;
  private: boolean;
  binding_secret: string;
  template_id: string;
  template_name: string;
  template_persona: string;
  template_tools: string[];
  template_model: string;
  template_starters: unknown;
}

const SELECT = `
  SELECT a.id, a.hub_agent_id, a.owner_user_id, a.persona, a.model, a.private, a.binding_secret,
         t.id AS template_id, t.name AS template_name, t.persona AS template_persona,
         t.tools AS template_tools, t.model AS template_model, t.starters AS template_starters
  FROM agents a
  JOIN templates t ON t.id = a.template_id
  WHERE a.deleted_at IS NULL`;

export class Registry {
  private readonly pool: Pool;
  private readonly key: Buffer;
  private readonly defaultModel: string;

  constructor(pool: Pool, key: Buffer, defaultModel: string) {
    this.pool = pool;
    this.key = key;
    this.defaultModel = defaultModel;
  }

  /** The agent an arriving event belongs to, or undefined if not ours. */
  async find(hubAgentId: string): Promise<AgentRecord | undefined> {
    const { rows } = await this.pool.query<Row>(`${SELECT} AND a.hub_agent_id = $1`, [hubAgentId]);
    const row = rows[0];
    return row ? this.toRecord(row) : undefined;
  }

  /** The same agent, by the id the job queue carries. */
  async findById(id: string): Promise<AgentRecord | undefined> {
    const { rows } = await this.pool.query<Row>(`${SELECT} AND a.id = $1`, [id]);
    const row = rows[0];
    return row ? this.toRecord(row) : undefined;
  }

  /**
   * The binding secret for an agent, decrypted. This is what the webhook
   * receiver verifies with, so it is asked for on every event: a hot path,
   * and a query on a unique index.
   */
  async secretFor(hubAgentId: string): Promise<string | undefined> {
    const { rows } = await this.pool.query<{ binding_secret: string }>(
      "SELECT binding_secret FROM agents WHERE hub_agent_id = $1 AND deleted_at IS NULL",
      [hubAgentId],
    );
    const row = rows[0];
    return row ? decrypt(this.key, row.binding_secret) : undefined;
  }

  /**
   * Record an agent this runtime now answers for. Called after the agent and
   * its binding have been made on the hub. Registering the same hub agent
   * again replaces the secret, which is what regenerating one means.
   */
  async register(input: {
    hubAgentId: string;
    ownerUserId: string;
    templateId: string;
    bindingSecret: string;
    persona?: string;
    model?: string;
    private?: boolean;
  }): Promise<AgentRecord> {
    await this.pool.query(
      `INSERT INTO agents (id, hub_agent_id, template_id, owner_user_id, persona, model,
                           binding_secret, private)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       ON CONFLICT (hub_agent_id) DO UPDATE
         SET binding_secret = EXCLUDED.binding_secret,
             persona        = EXCLUDED.persona,
             model          = EXCLUDED.model,
             template_id    = EXCLUDED.template_id,
             deleted_at     = NULL`,
      [
        randomUUID(),
        input.hubAgentId,
        input.templateId,
        input.ownerUserId,
        input.persona ?? "",
        input.model ?? "",
        encrypt(this.key, input.bindingSecret),
        input.private ?? true,
      ],
    );
    const record = await this.find(input.hubAgentId);
    if (!record) throw new Error(`registered ${input.hubAgentId} but could not read it back`);
    return record;
  }

  /** Add or update a ready-made definition. */
  async putTemplate(template: Template): Promise<void> {
    await this.pool.query(
      `INSERT INTO templates (id, name, persona, tools, model, starters)
       VALUES ($1, $2, $3, $4, $5, $6)
       ON CONFLICT (id) DO UPDATE
         SET name = EXCLUDED.name, persona = EXCLUDED.persona, tools = EXCLUDED.tools,
             model = EXCLUDED.model, starters = EXCLUDED.starters, updated_at = now()`,
      [
        template.id,
        template.name,
        template.persona,
        template.tools,
        template.model,
        JSON.stringify(template.starters),
      ],
    );
  }

  private toRecord(row: Row): AgentRecord {
    return {
      id: row.id,
      hubAgentId: row.hub_agent_id,
      ownerUserId: row.owner_user_id,
      template: {
        id: row.template_id,
        name: row.template_name,
        persona: row.template_persona,
        tools: row.template_tools ?? [],
        model: row.template_model,
        starters: Array.isArray(row.template_starters) ? (row.template_starters as string[]) : [],
      },
      // The person's words win over the template's, and a named model over
      // the default. Both fall through rather than being copied at creation,
      // so changing a template changes the agents built on it.
      persona: row.persona || row.template_persona,
      model: row.model || row.template_model || this.defaultModel,
      private: row.private,
    };
  }
}
