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
  /**
   * Which of pi's providers to run models through: `anthropic`, `fireworks`,
   * `openai`, `groq` and the rest of its catalogue. One provider at a time
   * for now; the person picking a model per agent (D68) is a later step.
   */
  modelProvider: string;
  /** That provider's key. Also read from the provider's own variable. */
  modelApiKey: string;
  /** What a template does not name. Ids are the provider's own. */
  defaultModel: string;
  /**
   * Tokens one person may spend in a day, across their agents. 0 means no
   * ceiling, which is for a laptop and not for a server.
   */
  dailyTokenLimit: number;
  logLevel: "debug" | "info" | "warn" | "error";
}

/** The variable each provider reads its own key from. */
export function keyVariable(provider: string): string {
  return `${provider.replace(/-/g, "_").toUpperCase()}_API_KEY`;
}

/** A sensible model when none is named, per provider. */
function defaultModelFor(provider: string): string {
  if (provider === "fireworks") return "accounts/fireworks/models/deepseek-v4-flash-0731";
  return "claude-sonnet-5";
}

/**
 * Read a .env beside the package, if there is one.
 *
 * Here rather than in main, so a script that loads config — registering an
 * agent, a one-off — is configured the same way the server is. A real
 * deployment sets the environment itself and ships no file.
 */
function loadEnvFile(): void {
  try {
    process.loadEnvFile(new URL("../.env", import.meta.url));
  } catch {
    // No .env is the normal case in production.
  }
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  if (env === process.env) loadEnvFile();
  const provider = env.RUNTIME_MODEL_PROVIDER ?? "anthropic";
  return {
    port: Number(env.RUNTIME_PORT ?? 8081),
    databaseUrl: required(env, "RUNTIME_DATABASE_URL"),
    hubUrl: (env.RUNTIME_HUB_URL ?? "http://localhost:8080").replace(/\/+$/, ""),
    publicUrl: (env.RUNTIME_PUBLIC_URL ?? "").replace(/\/+$/, ""),
    secretKey: parseKey(required(env, "RUNTIME_SECRET_KEY")),
    modelProvider: provider,
    // RUNTIME_MODEL_API_KEY wins, then the variable the provider itself reads,
    // so a machine already set up for one provider needs nothing new.
    modelApiKey: env.RUNTIME_MODEL_API_KEY ?? env[keyVariable(provider)] ?? "",
    defaultModel: env.RUNTIME_DEFAULT_MODEL ?? defaultModelFor(provider),
    dailyTokenLimit: Number(env.RUNTIME_DAILY_TOKEN_LIMIT ?? 200_000),
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
