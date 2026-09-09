/**
 * Every setting the runtime reads, in one place, checked once at startup.
 *
 * A process that will not work is better off refusing to start than finding
 * out at the first message: the person is already waiting by then.
 */

export interface Config {
  /** Where this process listens. The hub posts webhooks here. */
  port: number;
  /** Its own Postgres. Never the hub's. */
  databaseUrl: string;
  /** The hub's base URL — the only way the runtime reaches it. */
  hubUrl: string;
  /**
   * The public base of this service, used to build each agent's webhook URL.
   * The hub refuses anything that is not HTTPS, and refuses private and
   * loopback addresses, so a laptop uses socket mode instead.
   */
  publicUrl: string;
  /**
   * The key that encrypts binding secrets and, later, people's app tokens.
   * 32 bytes, hex or base64. Not in the database, so a stolen dump is
   * ciphertext.
   */
  secretKey: Buffer;
  /** The model provider's key. One provider to begin with. */
  anthropicApiKey: string;
  /** What a template does not name. */
  defaultModel: string;
  logLevel: "debug" | "info" | "warn" | "error";
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  return {
    port: Number(env.RUNTIME_PORT ?? 8081),
    databaseUrl: required(env, "RUNTIME_DATABASE_URL"),
    hubUrl: (env.RUNTIME_HUB_URL ?? "http://localhost:8080").replace(/\/+$/, ""),
    publicUrl: (env.RUNTIME_PUBLIC_URL ?? "").replace(/\/+$/, ""),
    secretKey: parseKey(required(env, "RUNTIME_SECRET_KEY")),
    anthropicApiKey: env.ANTHROPIC_API_KEY ?? "",
    defaultModel: env.RUNTIME_DEFAULT_MODEL ?? "claude-sonnet-5",
    logLevel: (env.RUNTIME_LOG_LEVEL as Config["logLevel"]) ?? "info",
  };
}

function required(env: NodeJS.ProcessEnv, name: string): string {
  const value = env[name];
  if (!value) throw new Error(`${name} is not set`);
  return value;
}

/**
 * Read the encryption key. Hex or base64, and exactly 32 bytes either way —
 * a short key is a weak one, and silently padding it would hide that.
 */
function parseKey(raw: string): Buffer {
  const key = /^[0-9a-fA-F]{64}$/.test(raw)
    ? Buffer.from(raw, "hex")
    : Buffer.from(raw, "base64");
  if (key.length !== 32) {
    throw new Error(
      `RUNTIME_SECRET_KEY must be 32 bytes (64 hex characters, or base64); got ${key.length}`,
    );
  }
  return key;
}
