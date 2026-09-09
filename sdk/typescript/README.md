# @cuckoo/agent

Connect an agent backend to a [Cuckoo](https://cuckoo.onl) hub from TypeScript.

Node 22 or newer. **No dependencies** — `fetch`, `WebSocket` and `crypto` are
all built in now.

```bash
npm install @cuckoo/agent
```

## One agent, on your laptop

Nothing to deploy and no public address: the backend holds a socket open to the
hub. This is what the Connect screen in the app hands you a secret for.

```ts
import { Agent } from "@cuckoo/agent";

const agent = new Agent({ secret: process.env.CUCKOO_SECRET!, hub: "http://localhost:8080" });

agent.onMessage(async (message, conversation) => {
  await conversation.send(`you said: ${message.text}`);
});

await agent.serve();
```

Replies that arrive word by word, which is what the app draws with a caret:

```ts
agent.onMessage(async (message, conversation) => {
  const reply = await conversation.stream();
  for await (const token of model(message.text)) await reply.append(token);
  await reply.finish({ buttons: [[{ id: "more", label: "Tell me more" }]] });
});
```

A button carries an **id** or a **url**, never both. An id comes back as a
message whose `action.buttonId` is that id; a url opens on the device and tells
you nothing.

## Many agents, behind one URL

A service answering for thousands of agents does not hold thousands of sockets.
It gives the hub a webhook URL per agent and verifies what arrives.

```ts
import { WebhookReceiver } from "@cuckoo/agent";
import { createServer } from "node:http";

const receiver = new WebhookReceiver({
  hub: "https://cuckoo.onl",
  // Your database. Return undefined for an agent you do not serve.
  secretFor: (agentId) => secrets.get(agentId),
  handlers: {
    onMessage: async (message, conversation) => {
      await conversation.send(`you said: ${message.text}`);
    },
  },
});

createServer(async (req, res) => {
  const agentId = req.url!.split("/").pop()!;   // POST /hooks/agt_…
  const body = await readBody(req);             // the raw bytes, unparsed
  try {
    await receiver.handle(agentId, req.headers, body);
    res.writeHead(200).end();
  } catch {
    res.writeHead(401).end();
  }
}).listen(8081);
```

Three things this gets right, and each of them is a way to be wrong:

- **The agent comes from the URL, not the body.** The secret is chosen before a
  byte of the body is trusted, and the body's own `agent_id` is then checked
  against it.
- **The body must be the bytes as they arrived.** Parse it into an object and
  re-encode it and the signature will not match: key order and spacing changed.
- **Answer within ten seconds.** The hub retries anything slower, so a handler
  that calls a model should write the event down, answer, and reply afterwards.

## Verifying by hand

```ts
import { verify, SignatureError } from "@cuckoo/agent";

try {
  verify(secret, headers, rawBody);
} catch (error) {
  if (error instanceof SignatureError) return reject();
}
```

The key is the **SHA-256 of the binding secret**, not the secret itself, and the
signature covers `"<timestamp>.<body>"`. The hub stores that hash to
authenticate you, so its plaintext is never at rest on the hub.

## Errors

Every refusal is a `ProtocolError` with the hub's own `code` — `no_binding`,
`not_participant`, `blocked`, `invalid_text`, `not_streaming`, `rate_limited` —
plus `message`, `field` and `requestId`.

```ts
import { ProtocolError } from "@cuckoo/agent";

try {
  await conversation.send(text);
} catch (error) {
  if (error instanceof ProtocolError && error.code === "blocked") stop(conversation.id);
  else throw error;
}
```

## Files

```ts
const bytes = await client.download(message.attachments[0]!.mediaId);
const uploaded = await client.upload("bill.png", bytes, { contentType: "image/png" });
await conversation.send("here it is", { attachments: [uploaded] });
```

Uploading is a separate step from sending, so a slow upload never holds a
message open. An upload nobody sends is removed after a day.

## Parity with the Python SDK

The same protocol, the same names where the languages allow it. Two deliberate
differences: this package has the **webhook receiver** Python does not, and
attachments are plain data whose bytes are fetched through the client rather
than through a method on the attachment.

## Development

```bash
npm install
npm run typecheck
npm test          # node's own test runner; no framework
npm run build
```

The webhook tests check against a signature produced by the hub's own Go code.
If either side ever changes how it signs, that test fails — which is the point.
