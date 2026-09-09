/**
 * The socket side: acknowledging, not answering the same event twice, and
 * what happens when the hub says do not come back.
 *
 * The hub is a fake socket here — the point is the agent's behaviour around
 * the connection, which is where a backend quietly loses or repeats messages.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { Agent } from "../src/agent.ts";

const SECRET = "bnd_sec_test";

/** A WebSocket the test drives: it records what was sent and pushes frames in. */
class FakeSocket {
  static last: FakeSocket | undefined;
  static opened = 0;

  readonly sent: Record<string, unknown>[] = [];
  private readonly listeners = new Map<string, ((event: unknown) => void)[]>();

  readonly url: string;

  constructor(url: string) {
    this.url = url;
    FakeSocket.last = this;
    FakeSocket.opened += 1;
    queueMicrotask(() => this.emit("open", {}));
  }

  addEventListener(type: string, fn: (event: unknown) => void): void {
    const list = this.listeners.get(type) ?? [];
    list.push(fn);
    this.listeners.set(type, list);
  }

  send(raw: string): void {
    this.sent.push(JSON.parse(raw) as Record<string, unknown>);
  }

  close(code = 1000, reason = ""): void {
    this.emit("close", { code, reason });
  }

  /** Push an event down to the agent, as the hub would. */
  deliver(envelope: Record<string, unknown>): void {
    this.emit("message", { data: JSON.stringify(envelope) });
  }

  private emit(type: string, event: unknown): void {
    for (const fn of this.listeners.get(type) ?? []) fn(event);
  }
}

function messageEvent(id: string, text = "hello"): Record<string, unknown> {
  return {
    id,
    type: "message.created",
    created_at: "2026-09-09T06:00:00Z",
    agent_id: "agt_1",
    data: {
      conversation: { id: "cnv_1", kind: "dm" },
      participants: [{ kind: "user", id: "usr_1", display_name: "Priya", is_me: false }],
      message: {
        id: `msg_${id}`,
        sender: { kind: "user", id: "usr_1", display_name: "Priya" },
        body: { text },
        status: "complete",
        created_at: "2026-09-09T06:00:00Z",
      },
    },
  };
}

function newAgent(handled: string[], options: { fail?: boolean } = {}) {
  const agent = new Agent({
    secret: SECRET,
    hub: "http://hub.test",
    WebSocketImpl: FakeSocket as unknown as typeof WebSocket,
  });
  agent.onMessage((message) => {
    handled.push(message.text);
    if (options.fail) throw new Error("this handler is broken");
  });
  return agent;
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 5));

test("serving refuses to start without a message handler", async () => {
  const agent = new Agent({ secret: SECRET, WebSocketImpl: FakeSocket as unknown as typeof WebSocket });
  await assert.rejects(() => agent.serve(), /onMessage/);
});

test("a handled event is acknowledged, and a repeat is answered once", async () => {
  const handled: string[] = [];
  const agent = newAgent(handled);
  const serving = agent.serve();
  await settle();

  const socket = FakeSocket.last!;
  assert.match(socket.url, /^ws:\/\/hub\.test\/v1\/agent\/socket$/);

  socket.deliver(messageEvent("evt_1", "first"));
  await settle();
  socket.deliver(messageEvent("evt_1", "first again"));
  await settle();

  // Handled once, acknowledged both times: the hub must be told to stop
  // redelivering, whether or not we had seen it before.
  assert.deepEqual(handled, ["first"]);
  assert.deepEqual(
    socket.sent.filter((f) => f.ack === "evt_1").length,
    2,
  );

  agent.stop();
  socket.close();
  await serving;
});

test("an event whose handler threw is not acknowledged, so the hub sends it again", async () => {
  const handled: string[] = [];
  const agent = newAgent(handled, { fail: true });
  const serving = agent.serve();
  await settle();

  const socket = FakeSocket.last!;
  socket.deliver(messageEvent("evt_2"));
  await settle();

  assert.deepEqual(handled, ["hello"]);
  assert.equal(socket.sent.filter((f) => f.ack === "evt_2").length, 0);

  agent.stop();
  socket.close();
  await serving;
});

test("a frame that is not an event is ignored rather than fatal", async () => {
  const handled: string[] = [];
  const agent = newAgent(handled);
  const serving = agent.serve();
  await settle();

  const socket = FakeSocket.last!;
  socket.deliver({ hello: "there" });
  socket.deliver({ reply_to_cid: "abc", ok: true });
  await settle();

  assert.deepEqual(handled, []);
  agent.stop();
  socket.close();
  await serving;
});

test("a binding that was taken over or revoked stops the agent for good", async () => {
  for (const code of [4001, 4002]) {
    FakeSocket.opened = 0;
    const agent = newAgent([]);
    const serving = agent.serve();
    await settle();

    FakeSocket.last!.close(code, "gone");
    await serving; // returns rather than reconnecting

    assert.equal(FakeSocket.opened, 1, `close code ${code} should not reconnect`);
  }
});
