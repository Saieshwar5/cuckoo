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

import { HubClient, InFlight } from "@cuckoo/agent";

import { Actions } from "./actions.ts";
import { Registry } from "./agents/registry.ts";
import { answer } from "./answer.ts";
import { createRuntimeServer } from "./api/server.ts";
import { loadConfig, type Config } from "./config.ts";
import { migrate, openPool } from "./db/pool.ts";
import type { Harness } from "./harness/harness.ts";
import { selectHarness } from "./harness/select.ts";
import { Jobs, Worker, type Job } from "./jobs.ts";
import { Proactive } from "./proactive.ts";
import { Routines } from "./routines.ts";
import { Scheduler } from "./scheduler.ts";
import { Turns } from "./memory/turns.ts";
import { Usage } from "./usage.ts";
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
  const routines = new Routines(pool);
  const actions = new Actions(pool);
  const proactive = new Proactive(pool);
  const tools = builtinTools({ routines, actions });
  const usage = new Usage(pool, config.dailyTokenLimit);
  const harness: Harness = selectHarness(config);
  // Runs in progress by conversation: what a person's stop cancels.
  const inflight = new InFlight();

  // Anything a previous process claimed and did not finish is due again.
  const recovered = await jobs.recoverAbandoned();
  if (recovered) console.log(`runtime: ${recovered} job(s) recovered from a previous run`);

  const worker = new Worker(jobs, (job) =>
    run(job, { registry, turns, harness, tools, usage, actions, routines, config, inflight }),
  );
  worker.start();

  const scheduler = new Scheduler({
    routines,
    registry,
    turns,
    harness,
    tools,
    usage,
    actions,
    proactive,
    inflight,
    hubUrl: config.hubUrl,
  });
  scheduler.start();

  const server = createRuntimeServer({ pool, registry, jobs, inflight, version: VERSION, hubUrl: config.hubUrl });
  await new Promise<void>((resolve) => server.listen(config.port, resolve));
  console.log(`runtime: listening on :${config.port}, hub ${config.hubUrl}`);

  const stop = async () => {
    console.log("runtime: stopping");
    await worker.stop();
    await scheduler.stop();
    server.close();
    await pool.end();
  };
  process.once("SIGINT", () => void stop().then(() => process.exit(0)));
  process.once("SIGTERM", () => void stop().then(() => process.exit(0)));

  return { server, worker, scheduler, pool, registry, jobs, turns, routines, stop };
}

interface RunDeps {
  registry: Registry;
  turns: Turns;
  harness: Harness;
  tools: Map<string, import("./harness/harness.ts").ToolFactory>;
  usage: Usage;
  actions: Actions;
  routines: Routines;
  config: Config;
  inflight: InFlight;
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
    message?: { id?: string; sender?: { kind?: string; id?: string }; body?: { text?: string } };
  };
  const conversationId = payload.conversation?.id;
  const message = payload.message;
  if (!conversationId || !message?.id) return;

  // Only a person's message is answered. An agent's own words coming back
  // would be a conversation with itself, and a costly one.
  if (message.sender?.kind !== "user") return;
  const userId = message.sender.id ?? "";
  if (!userId) return;
  const text = message.body?.text ?? "";
  if (!text.trim()) return;

  const secret = await deps.registry.secretFor(agent.hubAgentId);
  if (!secret) return;
  const client = new HubClient(secret, { hub: deps.config.hubUrl });

  // A tap on a button the agent offered is carried out by this code, not by
  // asking the model what the person probably meant. The model chose the
  // words; it does not get to choose whether the thing happens.
  const buttonId = (payload.message?.body as { action?: { button_id?: string } } | undefined)
    ?.action?.button_id;
  if (buttonId) {
    const done = await carryOut(buttonId, agent.id, conversationId, deps, client);
    if (done) return;
  }

  const running = deps.inflight.start(conversationId);
  try {
    await answer(
      { agent, conversationId, text, hubMessageId: message.id, userId },
      {
        turns: deps.turns,
        harness: deps.harness,
        tools: deps.tools,
        usage: deps.usage,
        client,
        signal: running.signal,
      },
    );
  } finally {
    deps.inflight.end(conversationId, running);
  }
}

// Started directly, rather than imported by a test.
if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop() ?? "")) {
  start().catch((error: unknown) => {
    console.error("runtime: could not start", error);
    process.exit(1);
  });
}

/**
 * Do what a tapped button said it would do.
 *
 * Returns whether the tap was ours: an unknown or expired id is not an error,
 * it is just a message, and the model can say something about it. A tap that
 * was ours never reaches the model at all — which is the point. What happens
 * is what the row said, not what a sentence can be talked into.
 */
async function carryOut(
  buttonId: string,
  agentId: string,
  conversationId: string,
  deps: RunDeps,
  client: HubClient,
): Promise<boolean> {
  const pending = await deps.actions.take(buttonId, agentId, conversationId);
  if (!pending) return false;

  const args = pending.arguments as Record<string, string>;
  try {
    if (pending.action === "create_routine") {
      const routine = await deps.routines.create({
        agentId,
        conversationId,
        userId: pending.userId,
        instruction: args.instruction ?? "",
        schedule: args.schedule ?? "",
        timezone: args.timezone,
      });
      await client.send(
        conversationId,
        `Done. Next: ${new Date(routine.nextRun).toLocaleString("en-IN", { timeZone: routine.timezone })}.`,
      );
      return true;
    }
    if (pending.action === "delete_routine") {
      await deps.routines.delete(args.id ?? "", agentId);
      await client.send(conversationId, "Stopped.");
      return true;
    }
  } catch (error) {
    console.error("runtime: a confirmed action failed", error);
    await client.send(conversationId, "That did not work. Nothing was changed.").catch(() => {});
    return true;
  }
  return false;
}
