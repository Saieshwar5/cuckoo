/**
 * The rows: who this runtime answers for, what it remembers, and the queue
 * between the hub's ten-second budget and a model that thinks for longer.
 *
 * These run against a real Postgres, migrated from empty. What they are really
 * checking is that one process can hold many agents belonging to many people
 * and never mix them up.
 */

import assert from "node:assert/strict";
import { randomBytes, randomUUID } from "node:crypto";
import { after, before, describe, test } from "node:test";

import { Registry } from "../src/agents/registry.ts";
import type { Pool } from "../src/db/pool.ts";
import { Jobs, Worker } from "../src/jobs.ts";
import { Turns } from "../src/memory/turns.ts";
import { BLANK, WEATHER } from "../src/templates.ts";
import { databaseAvailable, freshPool } from "./support/db.ts";

const KEY = randomBytes(32);
const available = await databaseAvailable();

describe("with a database", { skip: available ? false : "no postgres on :5433" }, () => {
  let pool: Pool;
  let registry: Registry;

  before(async () => {
    pool = await freshPool("rows");
    registry = new Registry(pool, KEY, "claude-sonnet-5");
    await registry.putTemplate(WEATHER);
    await registry.putTemplate(BLANK);
  });

  after(async () => {
    await pool?.end();
  });

  test("migrating twice applies nothing the second time", async () => {
    const { migrate } = await import("../src/db/pool.ts");
    assert.deepEqual(await migrate(pool), []);
  });

  test("an agent is found by the id the hub's URL names", async () => {
    await registry.register({
      hubAgentId: "agt_weather",
      ownerUserId: "usr_cuckoo",
      templateId: "weather",
      bindingSecret: "bnd_sec_weather",
      private: false,
    });

    const found = await registry.find("agt_weather");
    assert.equal(found?.hubAgentId, "agt_weather");
    assert.equal(found?.template.id, "weather");
    // The template's persona and tools come through, and the default model
    // fills in for a template that names none.
    assert.match(found!.persona, /weather assistant/);
    assert.deepEqual(found!.template.tools, ["weather"]);
    assert.equal(found!.model, "claude-sonnet-5");
    assert.equal(await registry.find("agt_nobody"), undefined);
  });

  test("the binding secret is encrypted at rest and comes back whole", async () => {
    await registry.register({
      hubAgentId: "agt_secret",
      ownerUserId: "usr_1",
      templateId: "blank",
      bindingSecret: "bnd_sec_the_real_one",
    });

    assert.equal(await registry.secretFor("agt_secret"), "bnd_sec_the_real_one");

    const { rows } = await pool.query<{ binding_secret: string }>(
      "SELECT binding_secret FROM agents WHERE hub_agent_id = 'agt_secret'",
    );
    // What is actually in the table is not the secret.
    assert.equal(rows[0]!.binding_secret.includes("the_real_one"), false);
    assert.match(rows[0]!.binding_secret, /^v1:/);
  });

  test("one person's agents and another's stay apart", async () => {
    await registry.register({
      hubAgentId: "agt_priya",
      ownerUserId: "usr_priya",
      templateId: "blank",
      bindingSecret: "bnd_sec_priya",
      persona: "You read Priya's mail.",
    });
    await registry.register({
      hubAgentId: "agt_ravi",
      ownerUserId: "usr_ravi",
      templateId: "blank",
      bindingSecret: "bnd_sec_ravi",
      persona: "You read Ravi's mail.",
    });

    const priya = await registry.find("agt_priya");
    const ravi = await registry.find("agt_ravi");
    assert.equal(priya!.ownerUserId, "usr_priya");
    assert.equal(ravi!.ownerUserId, "usr_ravi");
    assert.equal(priya!.persona, "You read Priya's mail.");
    assert.notEqual(await registry.secretFor("agt_priya"), await registry.secretFor("agt_ravi"));
  });

  test("registering the same agent again replaces its secret", async () => {
    await registry.register({
      hubAgentId: "agt_again",
      ownerUserId: "usr_1",
      templateId: "blank",
      bindingSecret: "bnd_sec_first",
    });
    await registry.register({
      hubAgentId: "agt_again",
      ownerUserId: "usr_1",
      templateId: "blank",
      bindingSecret: "bnd_sec_second",
    });
    assert.equal(await registry.secretFor("agt_again"), "bnd_sec_second");
  });

  test("turns are kept in full, and the window is the recent end of them", async () => {
    const agent = await registry.find("agt_weather");
    const turns = new Turns(pool);
    const conversation = `cnv_${randomUUID()}`;

    for (let i = 0; i < 60; i += 1) {
      await turns.record({
        agentId: agent!.id,
        conversationId: conversation,
        role: i % 2 === 0 ? "user" : "assistant",
        text: `message ${i}`,
      });
    }

    // Nothing is forgotten...
    assert.equal(await turns.count(agent!.id, conversation), 60);
    // ...but a run is shown the recent end, oldest first.
    const window = await turns.window(agent!.id, conversation, 10);
    assert.equal(window.length, 10);
    assert.equal(window[0]!.text, "message 50");
    assert.equal(window.at(-1)!.text, "message 59");
  });

  test("one conversation's memory is not another's", async () => {
    const agent = await registry.find("agt_weather");
    const turns = new Turns(pool);
    await turns.record({ agentId: agent!.id, conversationId: "cnv_a", role: "user", text: "in a" });
    await turns.record({ agentId: agent!.id, conversationId: "cnv_b", role: "user", text: "in b" });

    const a = await turns.window(agent!.id, "cnv_a");
    assert.deepEqual(a.map((t) => t.text), ["in a"]);
  });

  test("an event delivered twice becomes one job", async () => {
    const agent = await registry.find("agt_weather");
    const jobs = new Jobs(pool);
    const event = { agentId: agent!.id, eventId: `evt_${randomUUID()}`, type: "message.created", payload: {} };

    assert.equal(await jobs.enqueue(event), true);
    // The hub redelivers anything it never saw acknowledged. The second one
    // must not become a second answer to the same message.
    assert.equal(await jobs.enqueue(event), false);
  });

  test("a job is claimed once, done, and not claimed again", async () => {
    const agent = await registry.find("agt_weather");
    const jobs = new Jobs(pool);
    await pool.query("DELETE FROM jobs");
    await jobs.enqueue({
      agentId: agent!.id,
      eventId: `evt_${randomUUID()}`,
      type: "message.created",
      payload: { conversation: { id: "cnv_1" } },
    });

    const done: string[] = [];
    const worker = new Worker(jobs, async (job) => { done.push(job.eventId); });
    assert.equal(await worker.tick(), 1);
    assert.equal(done.length, 1);
    // Nothing left due.
    assert.equal(await worker.tick(), 0);
  });

  test("a job that fails is tried again later, and gives up in the end", async () => {
    const agent = await registry.find("agt_weather");
    const jobs = new Jobs(pool);
    await pool.query("DELETE FROM jobs");
    const eventId = `evt_${randomUUID()}`;
    await jobs.enqueue({ agentId: agent!.id, eventId, type: "message.created", payload: {} });

    const worker = new Worker(jobs, async () => { throw new Error("the provider is down"); });
    await worker.tick();

    const after1 = await status(pool, eventId);
    assert.equal(after1.status, "pending");
    assert.match(after1.last_error!, /provider is down/);
    // Waiting, not immediately due again.
    assert.equal(await worker.tick(), 0);

    // Past the last attempt it stops rather than retrying forever.
    await pool.query("UPDATE jobs SET attempts = 5, run_at = now() WHERE event_id = $1", [eventId]);
    await worker.tick();
    assert.equal((await status(pool, eventId)).status, "failed");
  });

  test("work abandoned by a process that died is due again", async () => {
    const agent = await registry.find("agt_weather");
    const jobs = new Jobs(pool);
    const eventId = `evt_${randomUUID()}`;
    await jobs.enqueue({ agentId: agent!.id, eventId, type: "message.created", payload: {} });
    await pool.query(
      "UPDATE jobs SET status = 'running', claimed_at = now() - interval '1 hour' WHERE event_id = $1",
      [eventId],
    );

    assert.equal(await jobs.recoverAbandoned(), 1);
    assert.equal((await status(pool, eventId)).status, "pending");
  });
});

async function status(pool: Pool, eventId: string) {
  const { rows } = await pool.query<{ status: string; last_error: string | null }>(
    "SELECT status, last_error FROM jobs WHERE event_id = $1",
    [eventId],
  );
  return rows[0]!;
}
