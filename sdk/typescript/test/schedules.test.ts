/**
 * Schedules from the backend's side: hearing what the person did, confirming
 * it, and sending for it.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { HubClient } from "../src/client.ts";
import { dispatch, parseEnvelope } from "../src/events.ts";
import type { ScheduleChange } from "../src/schedules.ts";

const SCHEDULE = {
  id: "sch_1",
  conversation_id: "cnv_1",
  title: "Weather in Hyderabad",
  instruction: "Weather in Hyderabad",
  cadence: { repeat: "daily", time: "07:00", timezone: "Asia/Kolkata" },
  status: "pending",
  created_by: "user",
  next_run_at: "2026-09-12T01:30:00Z",
  last_run_at: null,
  missed_at: null,
};

function hub() {
  const calls: { method: string; path: string; body: Record<string, unknown> }[] = [];
  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    const path = new URL(String(input)).pathname;
    const body = typeof init?.body === "string" ? (JSON.parse(init.body) as Record<string, unknown>) : {};
    calls.push({ method: init?.method ?? "GET", path, body });
    if (path.endsWith("/messages")) {
      return Response.json({ message: { id: "msg_2", schedule_id: body.schedule_id, body: { text: "31" } } }, { status: 201 });
    }
    return Response.json({ schedule: { ...SCHEDULE, status: body.status ?? "pending", title: body.title ?? SCHEDULE.title } });
  }) as unknown as typeof fetch;
  return { calls, client: new HubClient("bnd_sec_test", { hub: "http://hub.test", fetch: fetchImpl }) };
}

test("a schedule the person made reaches onSchedule, and is confirmed with a name", async () => {
  const { calls, client } = hub();
  const heard: ScheduleChange[] = [];
  const envelope = parseEnvelope({
    id: "evt_1",
    type: "schedule.requested",
    agent_id: "agt_1",
    data: { conversation: { id: "cnv_1", kind: "dm" }, schedule: SCHEDULE },
  });

  await dispatch(envelope, client, {
    onSchedule: async (change, conversation) => {
      heard.push(change);
      const confirmed = await conversation.schedules.confirm(change.schedule.id, "Morning weather");
      assert.equal(confirmed.status, "active");
    },
  });

  assert.equal(heard[0]?.type, "requested");
  assert.deepEqual(heard[0]?.schedule.cadence, { repeat: "daily", time: "07:00", timezone: "Asia/Kolkata" });
  assert.equal(heard[0]?.schedule.nextRunAt, "2026-09-12T01:30:00Z");
  assert.deepEqual(calls[0], {
    method: "PATCH",
    path: "/v1/agent/conversations/cnv_1/schedules/sch_1",
    body: { status: "active", title: "Morning weather" },
  });
});

test("a message says which schedule sent it", async () => {
  const { calls, client } = hub();
  const sent = await client.send("cnv_1", "31°C", { scheduleId: "sch_1" });
  assert.equal(calls[0]?.body.schedule_id, "sch_1");
  assert.equal(sent.scheduleId, "sch_1");
  await client.startStream("cnv_1", { scheduleId: "sch_1" });
  assert.equal(calls[1]?.body.schedule_id, "sch_1");
});
