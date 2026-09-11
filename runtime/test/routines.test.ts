/**
 * Speaking first, and the two things that must not be left to a prompt: a tap
 * before anything changes, and a ceiling on how often a person is spoken to.
 *
 * A model can be talked out of a rule in its persona. It cannot be talked out
 * of a row that is not there.
 */

import assert from "node:assert/strict";
import { randomBytes, randomUUID } from "node:crypto";
import { after, before, describe, test } from "node:test";

import { Actions } from "../src/actions.ts";
import { Registry } from "../src/agents/registry.ts";
import type { Pool } from "../src/db/pool.ts";
import { Proactive } from "../src/proactive.ts";
import { Routines, nextRun, validSchedule } from "../src/routines.ts";
import { BLANK, REMINDERS, TRANSLATOR, WEATHER } from "../src/templates.ts";
import { createRoutineTool } from "../src/tools/routines.ts";
import { databaseAvailable, freshPool } from "./support/db.ts";

const KEY = randomBytes(32);
const available = await databaseAvailable();

test("a schedule is read in its own zone, not the server's", () => {
  // 08:00 in Kolkata is 02:30 UTC. If this ever reads as 08:00 UTC, somebody
  // in Hyderabad is woken at half past one in the afternoon.
  const next = nextRun("0 8 * * *", "Asia/Kolkata", new Date("2026-09-09T00:00:00Z"));
  assert.equal(next.toISOString(), "2026-09-09T02:30:00.000Z");
});

test("a schedule that cannot be read is refused rather than guessed", () => {
  assert.equal(validSchedule("0 8 * * *", "Asia/Kolkata"), true);
  assert.equal(validSchedule("every morning please", "Asia/Kolkata"), false);
  assert.equal(validSchedule("0 8 * * *", "Mars/Olympus"), false);
});

describe("with a database", { skip: available ? false : "no postgres on :5433" }, () => {
  let pool: Pool;
  let registry: Registry;
  let routines: Routines;
  let actions: Actions;
  let agentId: string;

  before(async () => {
    pool = await freshPool("routines");
    registry = new Registry(pool, KEY, "claude-sonnet-5");
    for (const t of [WEATHER, TRANSLATOR, REMINDERS, BLANK]) await registry.putTemplate(t);
    const agent = await registry.register({
      hubAgentId: "agt_reminders",
      ownerUserId: "usr_cuckoo",
      templateId: "reminders",
      bindingSecret: "bnd_sec_reminders",
      private: false,
    });
    agentId = agent.id;
    routines = new Routines(pool);
    actions = new Actions(pool);
  });

  after(async () => {
    await pool?.end();
  });

  test("asking for a routine writes nothing until the person taps", async () => {
    const tool = createRoutineTool(
      { routines, actions },
      { agentId, conversationId: "cnv_1", userId: "usr_priya" },
    );

    const result = (await tool.execute({
      instruction: "tell me the weather",
      title: "Morning weather",
      repeat: "daily",
      time: "08:00",
    })) as { confirm?: { buttonId: string }; proposed?: { when: string } };

    // Said back in words before anything is agreed to.
    assert.equal(result.proposed?.when, "every day at 8:00 am");

    // An offer exists…
    assert.ok(result.confirm?.buttonId);
    // …and nothing is running.
    assert.deepEqual(await routines.listFor(agentId, "cnv_1"), []);

    // The tap is what creates it.
    const pending = await actions.take(result.confirm.buttonId, agentId, "cnv_1");
    assert.equal(pending?.action, "create_routine");
    assert.equal(pending?.arguments.instruction, "tell me the weather");
    assert.deepEqual(pending?.arguments.cadence, { repeat: "daily", time: "08:00", timezone: "Asia/Kolkata" });
  });

  test("a time that cannot be set is refused before anyone is asked", async () => {
    const tool = createRoutineTool({ routines, actions }, { agentId, conversationId: "cnv_1", userId: "usr_priya" });
    const bad = (await tool.execute({ instruction: "x", title: "x", repeat: "hourly", time: "08:00" })) as { error?: string };
    assert.ok(bad.error);
    const past = (await tool.execute({
      instruction: "x", title: "x", repeat: "once", time: "08:00", date: "2020-01-01",
    })) as { error?: string };
    assert.match(past.error ?? "", /already passed/);
  });

  test("a button is good once, however many times it is tapped", async () => {
    const buttonId = await actions.offer({
      agentId,
      conversationId: "cnv_1",
      userId: "usr_priya",
      action: "create_routine",
      arguments: { instruction: "x", schedule: "0 8 * * *" },
    });
    assert.ok(await actions.take(buttonId, agentId, "cnv_1"));
    // A double press, or a redelivered tap, must not do it twice.
    assert.equal(await actions.take(buttonId, agentId, "cnv_1"), undefined);
  });

  test("a button belonging to another conversation is not honoured here", async () => {
    const buttonId = await actions.offer({
      agentId,
      conversationId: "cnv_1",
      userId: "usr_priya",
      action: "create_routine",
      arguments: {},
    });
    assert.equal(await actions.take(buttonId, agentId, "cnv_other"), undefined);
  });

  test("an offer that has expired is gone", async () => {
    const buttonId = await actions.offer({
      agentId,
      conversationId: "cnv_1",
      userId: "usr_priya",
      action: "create_routine",
      arguments: {},
    });
    await pool.query("UPDATE pending_actions SET expires_at = now() - interval '1 minute'");
    assert.equal(await actions.take(buttonId, agentId, "cnv_1"), undefined);
  });

  test("a due routine is claimed once and moved on to its next time", async () => {
    const routine = await routines.create({
      agentId,
      conversationId: "cnv_due",
      userId: "usr_priya",
      instruction: "the weather",
      schedule: "0 8 * * *",
    });
    await pool.query("UPDATE routines SET next_run = now() - interval '1 minute' WHERE id = $1", [
      routine.id,
    ]);

    const claimed = await routines.claimDue();
    assert.equal(claimed.filter((r) => r.id === routine.id).length, 1);
    // A second pass must not run it again: the claim moved it forward.
    assert.equal((await routines.claimDue()).filter((r) => r.id === routine.id).length, 0);

    const [after] = await routines.listFor(agentId, "cnv_due");
    assert.ok(new Date(after!.nextRun) > new Date(), "it should be scheduled in the future");
  });

  test("a routine that keeps failing is paused rather than retried forever", async () => {
    const routine = await routines.create({
      agentId,
      conversationId: "cnv_fail",
      userId: "usr_priya",
      instruction: "something broken",
      schedule: "0 8 * * *",
    });
    assert.equal((await routines.recordFailure(routine.id, "no")).paused, false);
    assert.equal((await routines.recordFailure(routine.id, "no")).paused, false);
    // The third one stops it. Nobody is served by a reminder that fails daily.
    assert.equal((await routines.recordFailure(routine.id, "no")).paused, true);

    const [paused] = await routines.listFor(agentId, "cnv_fail");
    assert.equal(paused!.paused, true);
    // And a paused routine is not claimed.
    await pool.query("UPDATE routines SET next_run = now() - interval '1 minute' WHERE id = $1", [
      routine.id,
    ]);
    assert.equal((await routines.claimDue()).filter((r) => r.id === routine.id).length, 0);
  });

  test("a person is only spoken to so many times a day", async () => {
    const proactive = new Proactive(pool, 3);
    const userId = `usr_${randomUUID()}`;

    for (let i = 0; i < 3; i += 1) {
      assert.equal(await proactive.maySend(userId), true);
      await proactive.record({
        userId,
        agentId,
        conversationId: "cnv_1",
        reason: "routine:test",
      });
    }
    // The fourth is refused. A scheduler bug is somebody's phone at three in
    // the morning, and blocking the agent is the only remedy they have.
    assert.equal(await proactive.maySend(userId), false);
    // Somebody else's budget is their own.
    assert.equal(await proactive.maySend(`usr_${randomUUID()}`), true);
  });
});
