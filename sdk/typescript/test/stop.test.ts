/**
 * Saying what the agent is doing, and being stopped.
 *
 * The promise to a developer is that stop works even if they write nothing
 * for it: a handler is cancelled, its next write throws, and the event is
 * treated as done rather than retried.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { HubClient } from "../src/client.ts";
import { StoppedError } from "../src/errors.ts";
import { InFlight, dispatch, parseEnvelope, type StopRequest } from "../src/events.ts";

const SECRET = "bnd_sec_test";

interface Call {
  path: string;
  body: Record<string, unknown>;
}

function hub(refuse: (path: string) => { code: string; status: number } | undefined = () => undefined) {
  const calls: Call[] = [];
  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    const path = new URL(String(input)).pathname;
    const body = typeof init?.body === "string" ? (JSON.parse(init.body) as Record<string, unknown>) : {};
    calls.push({ path, body });
    const refused = refuse(path);
    if (refused) {
      return new Response(JSON.stringify({ error: { code: refused.code, message: "no" } }), {
        status: refused.status,
        headers: { "Content-Type": "application/json" },
      });
    }
    if (path.endsWith("/messages")) {
      return Response.json({
        message: { id: "msg_2", sender: { kind: "agent", id: "agt_1" }, body: {}, status: "streaming" },
      }, { status: 201 });
    }
    return new Response(null, { status: 204 });
  }) as unknown as typeof fetch;
  return { calls, client: new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl }) };
}

const messageEvent = parseEnvelope({
  id: "evt_1",
  type: "message.created",
  agent_id: "agt_1",
  data: {
    conversation: { id: "cnv_1", kind: "dm" },
    participants: [],
    message: { id: "msg_1", sender: { kind: "user", id: "usr_1" }, body: { text: "flights to Goa" } },
  },
});

const stopEvent = parseEnvelope({
  id: "evt_2",
  type: "stop.requested",
  agent_id: "agt_1",
  data: { conversation: { id: "cnv_1", kind: "dm" }, message_id: "msg_2" },
});

test("working says what the agent is doing, then clears it, however the work ends", async () => {
  const { calls, client } = hub();
  await dispatch(messageEvent, client, {
    onMessage: async (_m, conv) => {
      const found = await conv.working("Searching flights", async () => "3 flights");
      assert.equal(found, "3 flights");
      await assert.rejects(conv.thinking(() => Promise.reject(new Error("model down"))), /model down/);
    },
  });
  const activity = calls.filter((c) => c.path.endsWith("/activity")).map((c) => c.body);
  assert.deepEqual(activity, [
    { state: "working", label: "Searching flights" },
    { state: "idle" },
    { state: "thinking" },
    { state: "idle" },
  ]);
});

test("a stop cancels the running handler, ends it quietly, and then asks onStop", async () => {
  const { calls, client } = hub();
  const inflight = new InFlight();
  let signalled = false;
  let afterStop: unknown;
  const stops: StopRequest[] = [];
  const handlers = {
    onMessage: async (_m: unknown, conv: import("../src/conversation.ts").Conversation) => {
      conv.signal.addEventListener("abort", () => (signalled = true));
      try {
        // Work that would never finish on its own.
        await conv.working("Searching flights", () => new Promise(() => {}));
      } finally {
        afterStop = await conv.send("found them").catch((e: unknown) => e);
      }
    },
    onStop: async (stop: StopRequest) => {
      stops.push(stop);
    },
  };

  const running = dispatch(messageEvent, client, handlers, inflight);
  await new Promise((resolve) => setTimeout(resolve, 5));
  await dispatch(stopEvent, client, handlers, inflight);

  // The message's dispatch resolves: acknowledged, never retried.
  await running;
  assert.equal(signalled, true);
  assert.ok(afterStop instanceof StoppedError, "a write after the stop is refused");
  assert.deepEqual(stops, [{ conversationId: "cnv_1", messageId: "msg_2" }]);
  assert.equal(calls.filter((c) => c.path.endsWith("/messages")).length, 0, "nothing was sent after the stop");
  assert.equal(inflight.stop("cnv_1"), 0, "nothing is left running");
});

test("a stop felt only at the hub — another process pressed it — ends the handler just the same", async () => {
  const { client } = hub((path) => (path.endsWith("/append") ? { code: "stopped", status: 409 } : undefined));
  let reached = false;
  await dispatch(messageEvent, client, {
    onMessage: async (_m, conv) => {
      const reply = await conv.stream();
      await reply.append("Searching");
      reached = true;
    },
  });
  assert.equal(reached, false);

  const refused = await client.appendStream("msg_2", "x").catch((e: unknown) => e);
  assert.ok(refused instanceof StoppedError);
  assert.equal((refused as StoppedError).code, "stopped");
});

test("anything else a handler throws still fails the event, so the hub sends it again", async () => {
  const { client } = hub();
  await assert.rejects(
    dispatch(messageEvent, client, {
      onMessage: () => {
        throw new Error("this handler is broken");
      },
    }),
    /broken/,
  );
});
