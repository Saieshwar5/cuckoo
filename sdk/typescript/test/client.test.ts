/**
 * The HTTP side: what goes on the wire, and what a refusal becomes.
 *
 * The hub is a stub here. What is being checked is the shape of the requests
 * — field names the hub actually reads — and that an error envelope arrives as
 * a ProtocolError with the code intact.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { HubClient } from "../src/client.ts";
import { ProtocolError } from "../src/errors.ts";

const SECRET = "bnd_sec_test";

interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
}

function stub(responder: (call: Call) => { status?: number; body?: unknown }) {
  const calls: Call[] = [];
  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    const call: Call = {
      url: String(input),
      method: init?.method ?? "GET",
      headers: (init?.headers ?? {}) as Record<string, string>,
      body: typeof init?.body === "string" ? JSON.parse(init.body) : init?.body,
    };
    calls.push(call);
    const { status = 200, body = {} } = responder(call);
    // 204 means no content, and a Response refuses to carry one.
    if (status === 204) return new Response(null, { status });
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  }) as unknown as typeof fetch;
  return { calls, fetchImpl };
}

test("a secret that is not a binding secret is refused at construction", () => {
  assert.throws(() => new HubClient("ses_tok_a_person"), /bnd_sec_/);
});

test("sending puts the message where the hub expects it", async () => {
  const { calls, fetchImpl } = stub(() => ({
    body: {
      message: {
        id: "msg_1",
        sender: { kind: "agent", id: "agt_1", display_name: "Weather" },
        body: { text: "sunny" },
        status: "complete",
        created_at: "2026-09-09T06:00:00Z",
      },
    },
  }));
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });

  const message = await client.send("cnv_1", "sunny", {
    buttons: [[{ id: "more", label: "Tomorrow" }, { url: "https://example.com", label: "Open" }]],
    quickReplies: ["Thanks"],
    replyTo: "msg_0",
    idempotencyKey: "key-1",
  });

  assert.equal(message.id, "msg_1");
  assert.equal(message.text, "sunny");

  const call = calls[0]!;
  assert.equal(call.method, "POST");
  assert.equal(call.url, "http://hub.test/v1/agent/conversations/cnv_1/messages");
  assert.equal(call.headers.Authorization, `Bearer ${SECRET}`);
  const body = call.body as Record<string, unknown>;
  assert.equal(body.text, "sunny");
  assert.equal(body.idempotency_key, "key-1");
  assert.equal(body.reply_to, "msg_0");
  assert.deepEqual(body.quick_replies, [{ label: "Thanks" }]);
  // A choice carries an id, a link a url, and neither carries both.
  assert.deepEqual(body.buttons, [
    [
      { id: "more", label: "Tomorrow", style: "default" },
      { url: "https://example.com", label: "Open", style: "default" },
    ],
  ]);
});

test("an idempotency key is invented when none is given", async () => {
  const { calls, fetchImpl } = stub(() => ({ body: { message: { id: "msg_1" } } }));
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });
  await client.send("cnv_1", "hello");
  const body = calls[0]!.body as Record<string, string>;
  assert.match(body.idempotency_key!, /^[0-9a-f-]{36}$/);
});

test("a stream is started empty, appended to, and finished with its buttons", async () => {
  const { calls, fetchImpl } = stub((call) => {
    if (call.url.endsWith("/append")) return { status: 204 };
    return { body: { message: { id: "msg_s", status: "streaming" } } };
  });
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });

  const started = await client.startStream("cnv_1");
  await client.appendStream(started.id, "it will ");
  await client.appendStream(started.id, "rain");
  await client.finishStream(started.id, "cnv_1", { buttons: [[{ id: "week", label: "This week" }]] });

  assert.deepEqual(calls[0]!.body, { stream: true });
  assert.equal(calls[1]!.url, "http://hub.test/v1/agent/messages/msg_s/append");
  assert.deepEqual(calls[1]!.body, { text: "it will " });
  assert.equal(calls[3]!.url, "http://hub.test/v1/agent/messages/msg_s/finish");
  assert.deepEqual(calls[3]!.body, {
    buttons: [[{ id: "week", label: "This week", style: "default" }]],
  });
});

test("the hub's refusal arrives with its code, not its status", async () => {
  const { fetchImpl } = stub(() => ({
    status: 422,
    body: {
      error: {
        code: "invalid_text",
        message: "That message is too long.",
        field: "text",
        request_id: "req_9",
      },
    },
  }));
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });

  await assert.rejects(
    () => client.send("cnv_1", "x".repeat(9000)),
    (error: unknown) => {
      assert.ok(error instanceof ProtocolError);
      assert.equal(error.code, "invalid_text");
      assert.equal(error.field, "text");
      assert.equal(error.requestId, "req_9");
      assert.equal(error.status, 422);
      return true;
    },
  );
});

test("something that is not the hub still becomes a ProtocolError", async () => {
  const fetchImpl = (async () =>
    new Response("<html>502 Bad Gateway</html>", { status: 502 })) as unknown as typeof fetch;
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });
  await assert.rejects(() => client.send("cnv_1", "hi"), (error: unknown) => {
    assert.ok(error instanceof ProtocolError);
    assert.equal(error.code, "http_error");
    assert.match(error.message, /502/);
    return true;
  });
});

test("typing and history hit the paths the protocol names", async () => {
  const { calls, fetchImpl } = stub(() => ({ body: { messages: [] } }));
  const client = new HubClient(SECRET, { hub: "http://hub.test", fetch: fetchImpl });
  await client.typing("cnv_1", "start");
  await client.history("cnv_1", { limit: 20 });
  assert.equal(calls[0]!.url, "http://hub.test/v1/agent/conversations/cnv_1/typing");
  assert.deepEqual(calls[0]!.body, { state: "start" });
  assert.equal(calls[1]!.url, "http://hub.test/v1/agent/conversations/cnv_1/messages?limit=20");
});
