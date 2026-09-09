/**
 * The door the hub posts through.
 *
 * The contract being checked: a signed event becomes a job and a 200 in
 * milliseconds, an unsigned one becomes a 401 and nothing at all, and a
 * redelivery does not become a second answer. The model is never called here
 * — that is the whole point of the queue.
 */

import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { AddressInfo } from "node:net";
import { after, before, describe, test } from "node:test";

import { sign, signingKey } from "@cuckoo/agent";

import { Registry } from "../src/agents/registry.ts";
import { createRuntimeServer } from "../src/api/server.ts";
import type { Pool } from "../src/db/pool.ts";
import { Jobs } from "../src/jobs.ts";
import { BLANK, WEATHER } from "../src/templates.ts";
import { databaseAvailable, freshPool } from "./support/db.ts";
import { randomBytes } from "node:crypto";

const KEY = randomBytes(32);
const SECRET = "bnd_sec_the_weather_agent";
const available = await databaseAvailable();

describe("the webhook door", { skip: available ? false : "no postgres on :5433" }, () => {
  let pool: Pool;
  let jobs: Jobs;
  let base: string;
  let server: ReturnType<typeof createRuntimeServer>;

  before(async () => {
    pool = await freshPool("door");
    const registry = new Registry(pool, KEY, "claude-sonnet-5");
    await registry.putTemplate(WEATHER);
    await registry.putTemplate(BLANK);
    await registry.register({
      hubAgentId: "agt_weather",
      ownerUserId: "usr_cuckoo",
      templateId: "weather",
      bindingSecret: SECRET,
      private: false,
    });
    jobs = new Jobs(pool);
    server = createRuntimeServer({
      pool,
      registry,
      jobs,
      version: "test",
      hubUrl: "http://127.0.0.1:9",
    });
    await new Promise<void>((resolve) => server.listen(0, resolve));
    base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });

  after(async () => {
    server?.close();
    await pool?.end();
  });

  function event(id: string): string {
    return JSON.stringify({
      id,
      type: "message.created",
      created_at: new Date().toISOString(),
      agent_id: "agt_weather",
      data: {
        conversation: { id: "cnv_1", kind: "dm" },
        message: {
          id: "msg_1",
          sender: { kind: "user", id: "usr_1", display_name: "Priya" },
          body: { text: "will it rain?" },
        },
      },
    });
  }

  function post(agentId: string, body: string, secret = SECRET): Promise<Response> {
    const timestamp = String(Math.floor(Date.now() / 1000));
    return fetch(`${base}/hooks/${agentId}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Cuckoo-Event": "message.created",
        "X-Cuckoo-Timestamp": timestamp,
        "X-Cuckoo-Signature": sign(signingKey(secret), timestamp, body),
      },
      body,
    });
  }

  test("is alive, and says what it can reach", async () => {
    const response = await fetch(`${base}/healthz`);
    assert.equal(response.status, 200);
    // The hub is not running in this test, so it reports down while the
    // service itself is still up and answering — degraded, not dead.
    assert.deepEqual(await response.json(), {
      status: "degraded",
      version: "test",
      components: { postgres: "ok", hub: "down" },
    });
  });

  test("a signed event becomes a job", async () => {
    const id = `evt_${randomUUID()}`;
    const response = await post("agt_weather", event(id));

    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), { accepted: true });

    const { rows } = await pool.query<{ type: string; status: string }>(
      "SELECT type, status FROM jobs WHERE event_id = $1",
      [id],
    );
    assert.equal(rows[0]?.type, "message.created");
    assert.equal(rows[0]?.status, "pending");
  });

  test("the same event twice is one job and two happy answers", async () => {
    const body = event(`evt_${randomUUID()}`);
    const first = await post("agt_weather", body);
    const second = await post("agt_weather", body);

    assert.equal(first.status, 200);
    assert.deepEqual(await first.json(), { accepted: true });
    // The hub must be told to stop redelivering either way; only the first
    // one is work.
    assert.equal(second.status, 200);
    assert.deepEqual(await second.json(), { accepted: false });
  });

  test("an unsigned request is refused and writes nothing", async () => {
    const id = `evt_${randomUUID()}`;
    const response = await fetch(`${base}/hooks/agt_weather`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: event(id),
    });

    assert.equal(response.status, 401);
    const { rowCount } = await pool.query("SELECT 1 FROM jobs WHERE event_id = $1", [id]);
    assert.equal(rowCount, 0);
  });

  test("a request signed with the wrong secret is refused", async () => {
    const response = await post("agt_weather", event(`evt_${randomUUID()}`), "bnd_sec_not_it");
    assert.equal(response.status, 401);
  });

  test("an agent this runtime does not serve looks the same as a bad signature", async () => {
    // On purpose: a stranger probing must not learn which agents live here.
    const response = await post("agt_stranger", event(`evt_${randomUUID()}`));
    assert.equal(response.status, 401);
  });

  test("a body that names another agent than the URL is refused", async () => {
    const body = event(`evt_${randomUUID()}`).replace('"agt_weather"', '"agt_someone_else"');
    const response = await post("agt_weather", body);
    assert.equal(response.status, 401);
  });

  test("anything else is a 404", async () => {
    assert.equal((await fetch(`${base}/`)).status, 404);
  });
});
