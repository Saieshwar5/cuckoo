/**
 * The Management API client, against the shapes the hub actually returns.
 *
 * What matters here is the field names: this half of the protocol is what a
 * company's deploy script talks to, and a wrong key is a 422 at three in the
 * morning rather than a type error.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { Management } from "../src/management.ts";

const KEY = "mgt_tok_test";

interface Call {
  url: string;
  method: string;
  body: unknown;
  auth?: string;
}

function stub(responder: (call: Call) => unknown) {
  const calls: Call[] = [];
  const fetchImpl = (async (input: string | URL, init?: RequestInit) => {
    const headers = (init?.headers ?? {}) as Record<string, string>;
    const call: Call = {
      url: String(input),
      method: init?.method ?? "GET",
      body: typeof init?.body === "string" ? JSON.parse(init.body) : init?.body,
      auth: headers.Authorization,
    };
    calls.push(call);
    return new Response(JSON.stringify(responder(call) ?? {}), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as unknown as typeof fetch;
  return { calls, mgmt: new Management(KEY, { hub: "http://hub.test", fetch: fetchImpl }) };
}

const AGENT_WIRE = {
  id: "agt_1",
  handle: "weather",
  display_name: "Weather",
  description: "Run by Cuckoo.",
  starters: ["Weather in Hyderabad"],
  has_avatar: false,
  created_at: "2026-09-09T06:00:00Z",
  binding: {
    id: "bnd_1",
    mode: "webhook",
    webhook_url: "https://runtime.example/hooks/agt_1",
    status: "connected",
    last_seen_at: "2026-09-09T06:01:00Z",
  },
};

test("creating an agent sends the hub's field names", async () => {
  const { calls, mgmt } = stub(() => ({ agent: AGENT_WIRE }));

  const agent = await mgmt.createAgent({
    handle: "weather",
    displayName: "Weather",
    description: "Run by Cuckoo.",
    starters: ["Weather in Hyderabad"],
  });

  assert.equal(calls[0]!.url, "http://hub.test/v1/mgmt/agents");
  assert.equal(calls[0]!.auth, `Bearer ${KEY}`);
  assert.deepEqual(calls[0]!.body, {
    handle: "weather",
    display_name: "Weather",
    description: "Run by Cuckoo.",
    starters: ["Weather in Hyderabad"],
  });
  // …and what comes back is read into names TypeScript would have chosen.
  assert.equal(agent.displayName, "Weather");
  assert.equal(agent.binding?.mode, "webhook");
  assert.equal(agent.binding?.status, "connected");
  assert.equal(agent.binding?.webhookUrl, "https://runtime.example/hooks/agt_1");
});

test("an agent with no backend has no binding, not an empty one", async () => {
  const { mgmt } = stub(() => ({ agent: { ...AGENT_WIRE, binding: null } }));
  const agent = await mgmt.agent("agt_1");
  assert.equal(agent.binding, null);
});

test("connecting without a url is a socket, with one is a webhook", async () => {
  const { calls, mgmt } = stub(() => ({ secret: "bnd_sec_abc" }));

  assert.equal(await mgmt.connect("agt_1"), "bnd_sec_abc");
  assert.deepEqual(calls[0]!.body, { mode: "socket" });

  await mgmt.connect("agt_1", { webhookUrl: "https://runtime.example/hooks/agt_1" });
  assert.deepEqual(calls[1]!.body, {
    mode: "webhook",
    webhook_url: "https://runtime.example/hooks/agt_1",
  });
});

test("a binding that returns no secret is an error, not an empty string", async () => {
  // The secret is shown once. Silently handing back "" would store nothing
  // and fail later, somewhere else.
  const { mgmt } = stub(() => ({}));
  await assert.rejects(() => mgmt.connect("agt_1"), /no secret/);
});

test("a poster's code asks for nothing, and a per-customer one carries a reference", async () => {
  const { calls, mgmt } = stub(() => ({
    token: { id: "pt_1", max_uses: 1, use_count: 0 },
    code: "abc123",
    url: "https://cuckoo.onl/p/abc123",
    qr_png: "data:image/png;base64,iVBOR",
  }));

  // A poster: any number of people, no expiry. The hub has no "kind" field —
  // sending one is a 422, which is what the live hub taught this test.
  await mgmt.createCode("agt_1");
  assert.deepEqual(calls[0]!.body, {});

  const code = await mgmt.createCode("agt_1", { maxUses: 1, payload: { customer: "c-42" } });
  assert.deepEqual(calls[1]!.body, { max_uses: 1, payload: { customer: "c-42" } });

  // The envelope carries the link and the picture beside the token, and is
  // the only answer that ever contains the code itself.
  assert.equal(code.id, "pt_1");
  assert.equal(code.url, "https://cuckoo.onl/p/abc123");
  assert.equal(code.code, "abc123");
  assert.equal(code.maxUses, 1);
});

test("listing codes reads the hub's envelope", async () => {
  const { mgmt } = stub(() => ({
    tokens: [{ id: "pt_1", max_uses: null, use_count: 7, revoked_at: null, expires_at: null }],
  }));
  const [first] = await mgmt.codes("agt_1");
  assert.equal(first!.useCount, 7);
  assert.equal(first!.maxUses, null);
});

test("updating sends only what was named", async () => {
  const { calls, mgmt } = stub(() => ({ agent: AGENT_WIRE }));
  await mgmt.updateAgent("agt_1", { description: "New words." });
  assert.deepEqual(calls[0]!.body, { description: "New words." });
  assert.equal(calls[0]!.method, "PATCH");
});

test("deleting and disconnecting need no body and accept no content", async () => {
  const fetchImpl = (async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
  const mgmt = new Management(KEY, { hub: "http://hub.test", fetch: fetchImpl });
  await mgmt.deleteAgent("agt_1");
  await mgmt.disconnect("agt_1");
});
