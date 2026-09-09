/**
 * The runtime: one process, every agent.
 *
 * It receives events for thousands of agents on one URL, answers with a model
 * and tools, and holds nothing per agent between messages. Starting a second
 * copy of this process is how it scales; the job queue's SKIP LOCKED means the
 * two share the work rather than duplicating it.
 *
 * It reaches the hub only through the public protocol — the same agent and
 * management APIs any company uses. There is no private door, and if this
 * ever needs one, the protocol is what is incomplete.
 */

import { HubClient } from "@cuckoo/agent";

import { Registry } from "./agents/registry.ts";
import { answer } from "./answer.ts";
import { createRuntimeServer } from "./api/server.ts";
import { loadConfig, type Config } from "./config.ts";
import { migrate, openPool } from "./db/pool.ts";
import type { Harness } from "./harness/harness.ts";
import { selectHarness } from "./harness/select.ts";
import { Jobs, Worker, type Job } from "./jobs.ts";
import { Turns } from "./memory/turns.ts";
import { TEMPLATES } from "./templates.ts";
import { builtinTools } from "./tools/index.ts";

const VERSION = process.env.RUNTIME_VERSION ?? "dev";

export async function start(config: Config = loadConfig()) {
  const pool = openPool(config.databaseUrl);
  const applied = await migrate(pool);
  if (applied.length) console.log(`runtime: applied ${applied.join(", ")}`);

  const registry = new Registry(pool, config.secretKey, config.defaultModel);
  for (const template of TEMPLATES) await registry.putTemplate(template);

  const turns = new Turns(pool);
  const jobs = new Jobs(pool);
  const tools = builtinTools();
  const harness: Harness = selectHarness(config);

  // Anything a previous process claimed and did not finish is due again.
  const recovered = await jobs.recoverAbandoned();
  if (recovered) console.log(`runtime: ${recovered} job(s) recovered from a previous run`);

  const worker = new Worker(jobs, (job) =>
    run(job, { registry, turns, harness, tools, config }),
  );
  worker.start();

  const server = createRuntimeServer({ pool, registry, jobs, version: VERSION });
  await new Promise<void>((resolve) => server.listen(config.port, resolve));
  console.log(`runtime: listening on :${config.port}, hub ${config.hubUrl}`);

  const stop = async () => {
    console.log("runtime: stopping");
    await worker.stop();
    server.close();
    await pool.end();
  };
  process.once("SIGINT", () => void stop().then(() => process.exit(0)));
  process.once("SIGTERM", () => void stop().then(() => process.exit(0)));

  return { server, worker, pool, registry, jobs, turns, stop };
}

interface RunDeps {
  registry: Registry;
  turns: Turns;
  harness: Harness;
  tools: Map<string, import("./harness/harness.ts").Tool>;
  config: Config;
}

/**
 * One job: answer one message as one agent.
 *
 * Everything needed is loaded here and dropped at the end. A throw means the
 * job is retried — so this must be safe to run twice, which is why the reply
 * is the last thing that happens and nothing is written before the model runs.
 */
async function run(job: Job, deps: RunDeps): Promise<void> {
  if (job.type !== "message.created") return;

  const agent = await deps.registry.findById(job.agentId);
  if (!agent) return; // deleted while the job waited

  const payload = job.payload as {
    conversation?: { id?: string };
    message?: { id?: string; sender?: { kind?: string }; body?: { text?: string } };
  };
  const conversationId = payload.conversation?.id;
  const message = payload.message;
  if (!conversationId || !message?.id) return;

  // Only a person's message is answered. An agent's own words coming back
  // would be a conversation with itself, and a costly one.
  if (message.sender?.kind !== "user") return;
  const text = message.body?.text ?? "";
  if (!text.trim()) return;

  const secret = await deps.registry.secretFor(agent.hubAgentId);
  if (!secret) return;

  await answer(
    { agent, conversationId, text, hubMessageId: message.id },
    {
      turns: deps.turns,
      harness: deps.harness,
      tools: deps.tools,
      client: new HubClient(secret, { hub: deps.config.hubUrl }),
    },
  );
}

// Started directly, rather than imported by a test.
if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop() ?? "")) {
  start().catch((error: unknown) => {
    console.error("runtime: could not start", error);
    process.exit(1);
  });
}
