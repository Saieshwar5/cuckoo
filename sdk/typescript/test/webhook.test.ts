/**
 * The webhook receiver, checked against the hub's own signing.
 *
 * The vector below was produced by `delivery.Sign` in the Go hub. If either
 * side ever changes how it signs, this test fails — which is the only way to
 * catch two implementations drifting apart.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { SignatureError, WebhookReceiver, sign, signingKey, verify } from "../src/webhook.ts";
import type { Conversation } from "../src/conversation.ts";
import type { Message } from "../src/models.ts";

const SECRET = "bnd_sec_0123456789abcdefghjkmnpqrstvwxyz";
const TIMESTAMP = "1789000000";
const BODY =
  '{"id":"evt_01","type":"message.created","created_at":"2026-09-09T06:00:00Z","agent_id":"agt_01","data":{}}';
const SIGNATURE = "sha256=a30aa8770ab38a03bf1fd53fd232ac4eaf0a59914691230ca64890d94fb1e87c";

const NOW = Number(TIMESTAMP);

function headers(overrides: Record<string, string> = {}): Record<string, string> {
  return {
    "x-cuckoo-event": "message.created",
    "x-cuckoo-event-id": "evt_01",
    "x-cuckoo-timestamp": TIMESTAMP,
    "x-cuckoo-signature": SIGNATURE,
    ...overrides,
  };
}

test("signs exactly as the hub does", () => {
  assert.equal(sign(signingKey(SECRET), TIMESTAMP, BODY), SIGNATURE);
});

test("the signing key is the hash of the secret, not the secret", () => {
  // A backend that signed with the secret itself would agree with nothing.
  assert.notEqual(sign(Buffer.from(SECRET), TIMESTAMP, BODY), SIGNATURE);
  assert.equal(signingKey(SECRET).length, 32);
});

test("accepts what the hub sent", () => {
  verify(SECRET, headers(), BODY, { now: NOW });
});

test("refuses a body that was altered", () => {
  const tampered = BODY.replace("message.created", "message.deleted");
  assert.throws(() => verify(SECRET, headers(), tampered, { now: NOW }), SignatureError);
});

test("refuses the wrong secret", () => {
  assert.throws(
    () => verify("bnd_sec_somebodyelses", headers(), BODY, { now: NOW }),
    SignatureError,
  );
});

test("refuses a replay from long ago", () => {
  assert.throws(() => verify(SECRET, headers(), BODY, { now: NOW + 3600 }), SignatureError);
});

test("refuses a request with no signature at all", () => {
  const bare = { "x-cuckoo-event": "message.created" };
  assert.throws(() => verify(SECRET, bare, BODY, { now: NOW }), SignatureError);
});

test("reads headers from a Headers object too", () => {
  verify(SECRET, new Headers(headers()), BODY, { now: NOW });
});

test("one receiver serves many agents, each with its own secret", async () => {
  const secrets: Record<string, string> = { agt_01: SECRET, agt_99: "bnd_sec_another" };
  const receiver = new WebhookReceiver({
    secretFor: (agentId) => secrets[agentId],
    handlers: {},
    toleranceSeconds: 10 ** 9, // the vector's timestamp is fixed
  });

  const { envelope } = await receiver.handle("agt_01", headers(), BODY);
  assert.equal(envelope.id, "evt_01");
  assert.equal(envelope.agentId, "agt_01");

  // The same bytes posted to another agent's URL do not verify: that agent
  // has a different secret, so the signature cannot match.
  await assert.rejects(() => receiver.handle("agt_99", headers(), BODY), SignatureError);
  // And an agent this backend does not serve is refused before anything else.
  await assert.rejects(() => receiver.handle("agt_nope", headers(), BODY), SignatureError);
});

test("refuses a body naming an agent other than the URL it arrived at", async () => {
  // Signed correctly for agt_01's secret, but claiming to be agt_02 inside.
  const body = BODY.replace('"agent_id":"agt_01"', '"agent_id":"agt_02"');
  const signature = sign(signingKey(SECRET), TIMESTAMP, body);
  const receiver = new WebhookReceiver({
    secretFor: () => SECRET,
    handlers: {},
    toleranceSeconds: 10 ** 9,
  });
  await assert.rejects(
    () => receiver.handle("agt_01", headers({ "x-cuckoo-signature": signature }), body),
    SignatureError,
  );
});

test("routes a verified message to the handler", async () => {
  const body = JSON.stringify({
    id: "evt_02",
    type: "message.created",
    created_at: "2026-09-09T06:00:00Z",
    agent_id: "agt_01",
    data: {
      conversation: { id: "cnv_1", kind: "dm" },
      participants: [
        { kind: "user", id: "usr_1", display_name: "Priya", is_me: false },
        { kind: "agent", id: "agt_01", display_name: "Weather", is_me: true },
      ],
      message: {
        id: "msg_1",
        sender: { kind: "user", id: "usr_1", display_name: "Priya" },
        body: { text: "will it rain?" },
        status: "complete",
        created_at: "2026-09-09T06:00:00Z",
      },
    },
  });
  const timestamp = String(Math.floor(Date.now() / 1000));

  let seen: { message: Message; conversation: Conversation } | undefined;
  const receiver = new WebhookReceiver({
    secretFor: () => SECRET,
    handlers: {
      onMessage: (message, conversation) => {
        seen = { message, conversation };
      },
    },
  });

  await receiver.handle(
    "agt_01",
    headers({
      "x-cuckoo-timestamp": timestamp,
      "x-cuckoo-signature": sign(signingKey(SECRET), timestamp, body),
    }),
    body,
  );

  assert.equal(seen?.message.text, "will it rain?");
  assert.equal(seen?.message.sender.displayName, "Priya");
  assert.equal(seen?.message.eventId, "evt_02");
  assert.equal(seen?.conversation.id, "cnv_1");
  assert.equal(seen?.conversation.person?.displayName, "Priya");
});
