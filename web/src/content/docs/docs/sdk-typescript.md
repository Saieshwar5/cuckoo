---
title: TypeScript SDK
description: Reference for the @cuckoo/agent package — Agent, WebhookReceiver, Conversation, Message and Management.
---

The TypeScript SDK is three halves, which is one more than Python has.
`Agent` holds a socket open for one agent. `WebhookReceiver` verifies what the
hub posts and serves **any number of agents** behind one URL. `Management`
creates and manages agents, for deploy scripts and servers.

Node 22 or newer, and **no dependencies**: `fetch`, `WebSocket` and `crypto`
are all built in now.

## Install

The package is **not on npm yet**, so install it from a checkout.

```bash
npm install ./sdk/typescript
```

## One agent, on your laptop

Nothing to deploy and no public address: the backend holds a socket open to
the hub. This is what the Connect screen in the app hands you a secret for.

```ts
import { Agent } from "@cuckoo/agent";

const agent = new Agent({
  secret: process.env.CUCKOO_SECRET!,
  hub: "http://localhost:8080",
});

agent.onMessage(async (message, conversation) => {
  await conversation.send(`You said: ${message.text}`);
});

await agent.serve();
```

`serve()` reconnects on its own with a backoff, acknowledges every event, and
never answers the same event twice. It returns — rather than reconnecting —
when the hub says not to come back: another connection took over the binding,
or the binding was revoked.

### Handlers

```ts
agent.onMessage((message, conversation) => {});   // every message
agent.onJoin((conversation, code) => {});         // someone added the agent
agent.onLeave((conversationId, reason) => {});    // removed or blocked
```

A handler that throws is **not** acknowledged, so the hub sends the event
again. That is right for a backend that was briefly unable to answer, and
something to guard against for one that will never succeed.

## Many agents, behind one URL

A service answering for thousands of agents does not hold thousands of
sockets. It gives the hub a webhook URL per agent and verifies what arrives.

```ts
import { WebhookReceiver, SignatureError } from "@cuckoo/agent";
import { createServer } from "node:http";

const receiver = new WebhookReceiver({
  hub: "https://cuckoo.onl",
  // Your database. Return undefined for an agent you do not serve.
  secretFor: (agentId) => secrets.get(agentId),
  handlers: {
    onMessage: async (message, conversation) => {
      await conversation.send(`You said: ${message.text}`);
    },
  },
});

createServer(async (req, res) => {
  const agentId = req.url!.split("/").pop()!;   // POST /hooks/agt_…
  const body = await readRawBody(req);
  try {
    await receiver.handle(agentId, req.headers, body);
    res.writeHead(200).end();
  } catch (error) {
    res.writeHead(error instanceof SignatureError ? 401 : 422).end();
  }
}).listen(8081);
```

Three things this gets right, and each is a way to be wrong:

- **The agent comes from the URL, not the body.** The secret is chosen before
  a byte of the body is trusted, and the body's own `agent_id` is then checked
  against it. A body that disagrees with the URL it arrived at is refused.
- **The body must be the bytes as they arrived.** Parse it into an object and
  re-encode it and the signature will not match: key order and spacing changed.
- **Answer within ten seconds.** The hub retries anything slower. A handler
  that calls a model should write the event down, answer, and reply afterwards
  — otherwise a slow answer arrives twice.

### Verifying by hand

```ts
import { verify, sign, signingKey, SignatureError } from "@cuckoo/agent";

verify(secret, headers, rawBody);   // throws SignatureError
```

The key is the **SHA-256 of the binding secret**, not the secret itself, and
the signature covers `"<timestamp>.<body>"`. The hub stores that hash to
authenticate you, so the secret's plaintext is never at rest on the hub.

## Replying

```ts
await conversation.send("text");
await conversation.send("", { attachments: [uploaded] });
await conversation.send("Track it", {
  buttons: [[{ id: "status", label: "Where is it?" },
             { url: "https://example.com/track", label: "Open" }]],
  quickReplies: ["Thanks"],
  replyTo: message.id,
});
await conversation.typing("start");
```

A button carries an **id** or a **url**, never both — the hub refuses one that
carries two. An id comes back as a message whose `action.buttonId` is that id;
a url opens on the device and tells you nothing.

### Streaming

```ts
const reply = await conversation.stream();
for await (const token of model(message.text)) await reply.append(token);
await reply.finish({ buttons: [[{ id: "more", label: "Tell me more" }]] });
```

Buttons are attached at the end, not the start. A stream left idle for **30
seconds** is finished by the hub and marked truncated, so open it when you have
something to say rather than before you begin thinking.

## Files

```ts
const bytes = await agent.client.download(message.attachments[0]!.mediaId);
const uploaded = await agent.client.upload("bill.png", bytes, { contentType: "image/png" });
await conversation.send("here it is", { attachments: [uploaded] });
```

Uploading is a separate step from sending, so a slow upload never holds a
message open. An upload nobody sends is removed after a day.

## Management

The half a **server** uses: creating agents, connecting backends, and minting
the codes that hand them out. Authenticate with an API key (`mgt_tok_…`) or a
person's session token — never a binding secret, which speaks for one agent and
is refused here.

```ts
import { Management } from "@cuckoo/agent";

const mgmt = new Management(process.env.CUCKOO_KEY!, { hub: "https://cuckoo.onl" });

const agent = await mgmt.createAgent({
  handle: "support",
  displayName: "Acme Support",
  description: "Orders, refunds and delivery.",
  starters: ["Where is my order?", "I want a refund"],
});

// A webhook backend. Without a url it is a socket instead.
const secret = await mgmt.connect(agent.id, {
  webhookUrl: `https://acme.example/hooks/${agent.id}`,
});
// Shown once. Store it now.

const code = await mgmt.createCode(agent.id);
console.log(code.url);    // put this on a poster
console.log(code.qrPng);  // …or this straight into a page
```

| Method | What |
| --- | --- |
| `createAgent`, `agents`, `agent`, `updateAgent`, `deleteAgent` | The identity |
| `connect`, `disconnect` | The backend behind it. `connect` returns the secret, once |
| `createCode`, `codes`, `revokeCode`, `codePicture` | The codes that hand it out |
| `uploadAvatar` | A picture the agent wears |

**Codes.** Leave everything out for a poster: any number of people, no expiry —
that is what makes an agent public. Pass `maxUses: 1` with your own reference in
`payload` for a code minted per customer; the payload comes back on the join
event, before they say a word.

The code itself exists in full exactly once, in the answer that minted it — the
hub keeps only its hash. `codes()` lists what exists without the secrets, and
`codePicture()` needs the code handed back to draw it again.

## Errors

Every refusal is a `ProtocolError` carrying the hub's own `code`, plus
`message`, `field` and `requestId`.

```ts
import { ProtocolError } from "@cuckoo/agent";

try {
  await conversation.send(text);
} catch (error) {
  if (error instanceof ProtocolError && error.code === "blocked") stop(conversation.id);
  else throw error;
}
```

Codes you will meet most: `no_binding`, `not_participant`, `blocked`,
`invalid_text`, `not_streaming`, `rate_limited`, `handle_taken`.

## Differences from the Python SDK

The same protocol and the same names where the languages allow it. Three
deliberate differences:

- This package has the **webhook receiver**; Python does not.
- Attachments are plain data; their bytes are fetched through the client rather
  than through a method on the attachment.
- `Management` is asynchronous here, because there is no synchronous `fetch`.
