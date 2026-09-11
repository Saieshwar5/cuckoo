/**
 * Routines and the app's schedules, kept in step.
 *
 * The routine is what runs; the schedule is what the person sees, pauses and
 * deletes. If these drift, a schedule on somebody's screen never fires, or
 * one they deleted goes on messaging them.
 */

import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { after, before, describe, test } from "node:test";

import type { HubClient, Schedule, ScheduleChange } from "@cuckoo/agent";

import { Registry } from "../src/agents/registry.ts";
import type { Pool } from "../src/db/pool.ts";
import { Routines } from "../src/routines.ts";
import { applyChange, mirror } from "../src/schedules.ts";
import { REMINDERS, WEATHER } from "../src/templates.ts";
import { databaseAvailable, freshPool } from "./support/db.ts";

const available = await databaseAvailable();

/** A hub that records what it was told about schedules. */
class StubHub {
  readonly calls: string[] = [];
  async createSchedule(_c: string, input: { title: string }): Promise<Schedule> {
    this.calls.push(`create:${input.title}`);
    return { ...schedule("sch_made"), title: input.title };
  }
  async updateSchedule(_c: string, id: string, change: { status?: string }): Promise<Schedule> {
    this.calls.push(`update:${id}:${change.status}`);
    return schedule(id);
  }
  async deleteSchedule(_c: string, id: string): Promise<void> {
    this.calls.push(`delete:${id}`);
  }
  async send(_c: string, text: string): Promise<{ id: string }> {
    this.calls.push(`send:${text}`);
    return { id: "msg_1" };
  }
  asClient(): HubClient {
    return this as unknown as HubClient;
  }
}

function schedule(id: string, over: Partial<Schedule> = {}): Schedule {
  return {
    id,
    conversationId: "cnv_1",
    title: "Weather in Hyderabad",
    instruction: "tell me today's weather in Hyderabad",
    cadence: { repeat: "daily", time: "07:00", timezone: "Asia/Kolkata" },
    status: "pending",
    createdBy: "user",
    nextRunAt: null,
    lastRunAt: null,
    missedAt: null,
    ...over,
  };
}

describe("with a database", { skip: available ? false : "no postgres on :5433" }, () => {
  let pool: Pool;
  let routines: Routines;
  let agentId: string;
  const where = () => ({ agentId, conversationId: "cnv_1", userId: "usr_priya" });

  before(async () => {
    pool = await freshPool("schedules");
    const registry = new Registry(pool, randomBytes(32), "claude-sonnet-5");
    for (const t of [WEATHER, REMINDERS]) await registry.putTemplate(t);
    agentId = (
      await registry.register({
        hubAgentId: "agt_weather",
        ownerUserId: "usr_cuckoo",
        templateId: "weather",
        bindingSecret: "bnd_sec_weather",
        private: false,
      })
    ).id;
    routines = new Routines(pool);
  });

  after(async () => {
    await pool?.end();
  });

  test("a schedule made in the app becomes a routine, and is confirmed", async () => {
    const hub = new StubHub();
    const change: ScheduleChange = { type: "requested", schedule: schedule("sch_1") };

    await applyChange(change, where(), hub.asClient(), routines);

    const routine = await routines.findByHubId("sch_1");
    assert.equal(routine?.schedule, "0 7 * * *");
    assert.equal(routine?.userId, "usr_priya");
    assert.equal(routine?.instruction, "tell me today's weather in Hyderabad");
    assert.deepEqual(hub.calls, ["update:sch_1:active"]);

    // Heard twice — the hub redelivers — it is still one routine.
    await applyChange({ type: "updated", schedule: schedule("sch_1", { status: "active" }) }, where(), hub.asClient(), routines);
    assert.equal((await routines.listFor(agentId, "cnv_1")).filter((r) => r.hubScheduleId === "sch_1").length, 1);
  });

  test("paused, resumed, changed and deleted in the app, so is the routine", async () => {
    const hub = new StubHub();
    await applyChange({ type: "requested", schedule: schedule("sch_2") }, where(), hub.asClient(), routines);

    await applyChange({ type: "updated", schedule: schedule("sch_2", { status: "paused" }) }, where(), hub.asClient(), routines);
    assert.equal((await routines.findByHubId("sch_2"))?.paused, true);

    await applyChange({ type: "updated", schedule: schedule("sch_2", { status: "active" }) }, where(), hub.asClient(), routines);
    assert.equal((await routines.findByHubId("sch_2"))?.paused, false);

    const earlier = schedule("sch_2", { cadence: { repeat: "weekdays", time: "06:30", timezone: "Asia/Kolkata" } });
    await applyChange({ type: "updated", schedule: earlier }, where(), hub.asClient(), routines);
    assert.equal((await routines.findByHubId("sch_2"))?.schedule, "30 6 * * 1-5");
    assert.equal(hub.calls.at(-1), "update:sch_2:active", "a change is confirmed again");

    await applyChange({ type: "deleted", schedule: schedule("sch_2") }, where(), hub.asClient(), routines);
    assert.equal(await routines.findByHubId("sch_2"), undefined);
  });

  test("a paused schedule moves with its person's clock and stays paused", async () => {
    const hub = new StubHub();
    await applyChange({ type: "requested", schedule: schedule("sch_tz") }, where(), hub.asClient(), routines);
    await applyChange({ type: "updated", schedule: schedule("sch_tz", { status: "paused" }) }, where(), hub.asClient(), routines);

    const london = { repeat: "daily" as const, time: "07:00", timezone: "Europe/London" };
    await applyChange({ type: "updated", schedule: schedule("sch_tz", { status: "paused", cadence: london }) }, where(), hub.asClient(), routines);

    const routine = await routines.findByHubId("sch_tz");
    assert.equal(routine?.timezone, "Europe/London");
    assert.equal(routine?.paused, true);
  });

  test("a time it cannot hold is declined, and the person told", async () => {
    const hub = new StubHub();
    const odd = schedule("sch_3", { cadence: { repeat: "weekly", time: "07:00", timezone: "Asia/Kolkata" } });
    await applyChange({ type: "requested", schedule: odd }, where(), hub.asClient(), routines);
    assert.equal(await routines.findByHubId("sch_3"), undefined);
    assert.equal(hub.calls[0], "delete:sch_3");
    assert.match(hub.calls[1] ?? "", /^send:/);
  });

  test("a routine made in the chat is shown in the app", async () => {
    const hub = new StubHub();
    const routine = await routines.create({
      agentId,
      conversationId: "cnv_1",
      userId: "usr_priya",
      instruction: "tell me today's weather in Hyderabad",
      schedule: "0 7 * * *",
      title: "Morning weather",
    });
    assert.equal(await mirror(routine, { repeat: "daily", time: "07:00", timezone: "Asia/Kolkata" }, hub.asClient(), routines), true);
    assert.deepEqual(hub.calls, ["create:Morning weather"]);
    assert.equal((await routines.findByHubId("sch_made"))?.id, routine.id);
  });
});
