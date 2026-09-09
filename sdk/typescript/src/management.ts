/**
 * The Management API: creating agents, connecting backends, and minting the
 * codes that hand them out.
 *
 * This is the half of the protocol a *server* uses. A company's deploy script
 * creates its agent here and hands out a QR code; a service that runs agents
 * for other people creates one per person. The app uses these same routes
 * internally, which is why there is nothing here it can do that you cannot.
 *
 * Authenticate with an API key (`mgt_tok_…`), which belongs to an account and
 * can be rotated, or with a person's session token. Never a binding secret:
 * that speaks for one agent and is refused here.
 */

import { raiseForStatus } from "./errors.ts";
import { DEFAULT_HUB } from "./models.ts";

export interface AgentInfo {
  id: string;
  handle: string;
  displayName: string;
  description: string;
  starters: string[];
  hasAvatar: boolean;
  createdAt: string;
  /** How its backend is reached, or null when nothing is connected. */
  binding: BindingInfo | null;
}

export interface BindingInfo {
  id: string;
  mode: "socket" | "webhook";
  webhookUrl: string | null;
  status: "idle" | "connected" | "unreachable";
  lastSeenAt: string | null;
}

export interface Code {
  id: string;
  /** The link behind the QR: `https://<hub>/p/<code>`. */
  url: string;
  /** The code itself. The hub keeps only its hash, so this is the one time it exists in full. */
  code: string;
  /** The QR as a data URL, ready to put on a page. */
  qrPng: string;
  /** How many people may use it. Null for a poster: any number. */
  maxUses: number | null;
  useCount: number;
}

/** A code as it is listed later: what it is, not what it says. */
export interface CodeSummary {
  id: string;
  maxUses: number | null;
  useCount: number;
  revokedAt: string | null;
  expiresAt: string | null;
}

export interface CreateAgentInput {
  handle: string;
  displayName: string;
  description?: string;
  /** A few things it suggests saying first, shown as chips in an empty chat. */
  starters?: string[];
  /** A media id from `uploadAvatar`. */
  avatarMediaId?: string;
}

export interface UpdateAgentInput {
  displayName?: string;
  description?: string;
  starters?: string[];
  avatarMediaId?: string;
}

export interface ManagementOptions {
  hub?: string;
  timeoutMs?: number;
  fetch?: typeof fetch;
}

export class Management {
  readonly hub: string;
  private readonly key: string;
  private readonly timeoutMs: number;
  private readonly doFetch: typeof fetch;

  constructor(key: string, options: ManagementOptions = {}) {
    this.key = key;
    this.hub = (options.hub ?? DEFAULT_HUB).replace(/\/+$/, "");
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.doFetch = options.fetch ?? globalThis.fetch;
  }

  // -- agents --------------------------------------------------------------

  async createAgent(input: CreateAgentInput): Promise<AgentInfo> {
    const data = await this.call("POST", "/agents", {
      handle: input.handle,
      display_name: input.displayName,
      description: input.description ?? "",
      ...(input.starters ? { starters: input.starters } : {}),
      ...(input.avatarMediaId ? { avatar_media_id: input.avatarMediaId } : {}),
    });
    return toAgent(data.agent);
  }

  async agents(): Promise<AgentInfo[]> {
    const data = await this.call("GET", "/agents");
    return (Array.isArray(data.agents) ? data.agents : []).map(toAgent);
  }

  async agent(agentId: string): Promise<AgentInfo> {
    return toAgent((await this.call("GET", `/agents/${agentId}`)).agent);
  }

  async updateAgent(agentId: string, input: UpdateAgentInput): Promise<AgentInfo> {
    const body: Record<string, unknown> = {};
    if (input.displayName !== undefined) body.display_name = input.displayName;
    if (input.description !== undefined) body.description = input.description;
    if (input.starters !== undefined) body.starters = input.starters;
    if (input.avatarMediaId !== undefined) body.avatar_media_id = input.avatarMediaId;
    return toAgent((await this.call("PATCH", `/agents/${agentId}`, body)).agent);
  }

  /** The agent goes; its conversations and their history go with it. */
  async deleteAgent(agentId: string): Promise<void> {
    await this.call("DELETE", `/agents/${agentId}`);
  }

  // -- bindings ------------------------------------------------------------

  /**
   * Point an agent at a backend, and return the secret that speaks for it.
   *
   * The secret is shown **once**: store it now or set the binding again. With
   * no `webhookUrl` the binding is a socket, which is what a backend on a
   * laptop uses; with one, the hub posts events to it and refuses anything
   * that is not HTTPS or that resolves to a private address.
   */
  async connect(agentId: string, options: { webhookUrl?: string } = {}): Promise<string> {
    const data = await this.call("POST", `/agents/${agentId}/binding`, {
      mode: options.webhookUrl ? "webhook" : "socket",
      ...(options.webhookUrl ? { webhook_url: options.webhookUrl } : {}),
    });
    const secret = typeof data.secret === "string" ? data.secret : "";
    if (!secret) throw new Error("the hub returned no secret");
    return secret;
  }

  /** Take the backend away. The agent stays, and so does its history. */
  async disconnect(agentId: string): Promise<void> {
    await this.call("DELETE", `/agents/${agentId}/binding`);
  }

  // -- codes ---------------------------------------------------------------

  /**
   * Mint a code that adds this agent to whoever scans it.
   *
   * Leave everything out for a poster: any number of people, no expiry — that
   * is what makes an agent public. Pass `maxUses: 1` with your own reference
   * in `payload` for a code minted per customer; the payload comes back on the
   * join event, before they say a word, which is how a poster's anonymous scan
   * becomes a person you already know.
   */
  async createCode(
    agentId: string,
    options: { payload?: unknown; maxUses?: number; expiresInSeconds?: number } = {},
  ): Promise<Code> {
    const data = await this.call("POST", `/agents/${agentId}/pair-tokens`, {
      ...(options.payload !== undefined ? { payload: options.payload } : {}),
      ...(options.maxUses !== undefined ? { max_uses: options.maxUses } : {}),
      ...(options.expiresInSeconds ? { expires_in: options.expiresInSeconds } : {}),
    });
    return toCode(data);
  }

  /**
   * The agent's codes, newest first, revoked ones included. The secrets are
   * not here — the hub keeps only their hashes, so a code exists in full
   * exactly once, in the answer that minted it.
   */
  async codes(agentId: string): Promise<CodeSummary[]> {
    const data = await this.call("GET", `/agents/${agentId}/pair-tokens`);
    return (Array.isArray(data.tokens) ? data.tokens : []).map((raw) => {
      const w = (raw ?? {}) as Record<string, unknown>;
      return {
        id: String(w.id ?? ""),
        maxUses: (w.max_uses as number | null) ?? null,
        useCount: Number(w.use_count ?? 0),
        revokedAt: (w.revoked_at as string | null) ?? null,
        expiresAt: (w.expires_at as string | null) ?? null,
      };
    });
  }

  /** Stop sharing. People who already added the agent keep it. */
  async revokeCode(agentId: string, codeId: string): Promise<void> {
    await this.call("DELETE", `/agents/${agentId}/pair-tokens/${codeId}`);
  }

  /**
   * The link and the QR for a code you already have.
   *
   * The `code` itself must be handed back: the hub keeps only its hash, so it
   * cannot draw a picture of something it cannot read. Keep the `code` from
   * `createCode` if you will want the picture again — or simply keep `qrPng`,
   * which that answer already gave you.
   */
  async codePicture(
    agentId: string,
    codeId: string,
    code: string,
  ): Promise<{ url: string; qrPng: string }> {
    const query = new URLSearchParams({ code });
    const data = await this.call("GET", `/agents/${agentId}/pair-tokens/${codeId}/qr?${query}`);
    return { url: String(data.url ?? ""), qrPng: String(data.qr_png ?? "") };
  }

  // -- media ---------------------------------------------------------------

  /** Publish a picture an agent can wear, and return its media id. */
  async uploadAvatar(fileName: string, bytes: Uint8Array | Blob, contentType: string): Promise<string> {
    const form = new FormData();
    const blob = bytes instanceof Blob ? bytes : new Blob([bytes as BlobPart], { type: contentType });
    form.append("file", blob, fileName);
    const response = await this.doFetch(`${this.hub}/v1/mgmt/media`, {
      method: "POST",
      headers: this.headers(),
      body: form,
    });
    await raiseForStatus(response);
    const data = (await response.json()) as { media?: { id?: string } };
    return data.media?.id ?? "";
  }

  // -- plumbing ------------------------------------------------------------

  private headers(): Record<string, string> {
    return { Authorization: `Bearer ${this.key}` };
  }

  private async call(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<Record<string, unknown>> {
    const response = await this.doFetch(`${this.hub}/v1/mgmt${path}`, {
      method,
      headers: {
        ...this.headers(),
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    await raiseForStatus(response);
    if (response.status === 204) return {};
    const text = await response.text();
    return text ? (JSON.parse(text) as Record<string, unknown>) : {};
  }
}

function toAgent(raw: unknown): AgentInfo {
  const w = (raw ?? {}) as Record<string, unknown>;
  const binding = w.binding as Record<string, unknown> | null | undefined;
  return {
    id: String(w.id ?? ""),
    handle: String(w.handle ?? ""),
    displayName: String(w.display_name ?? ""),
    description: String(w.description ?? ""),
    starters: Array.isArray(w.starters) ? (w.starters as string[]) : [],
    hasAvatar: Boolean(w.has_avatar),
    createdAt: String(w.created_at ?? ""),
    binding: binding
      ? {
          id: String(binding.id ?? ""),
          mode: (binding.mode === "webhook" ? "webhook" : "socket") as BindingInfo["mode"],
          webhookUrl: (binding.webhook_url as string | null) ?? null,
          status: (binding.status ?? "idle") as BindingInfo["status"],
          lastSeenAt: (binding.last_seen_at as string | null) ?? null,
        }
      : null,
  };
}

/**
 * A minted code. The envelope carries the link and the picture beside the
 * token, and is the only response that ever contains the secret.
 */
function toCode(raw: unknown): Code {
  const w = (raw ?? {}) as Record<string, unknown>;
  const token = (w.token ?? {}) as Record<string, unknown>;
  return {
    id: String(token.id ?? ""),
    url: String(w.url ?? ""),
    code: String(w.code ?? ""),
    qrPng: String(w.qr_png ?? ""),
    maxUses: (token.max_uses as number | null) ?? null,
    useCount: Number(token.use_count ?? 0),
  };
}
