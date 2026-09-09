/**
 * The reference agent: it says what you said.
 *
 * The same agent as `examples/echo/echo.py`, in TypeScript, so the two SDKs
 * can be checked against each other by running both against one hub.
 *
 *   export CUCKOO_SECRET=bnd_sec_...   # from setting a socket binding
 *   export CUCKOO_HUB=http://localhost:8080
 *   npm run echo
 */

import { Agent } from "@cuckoo/agent";

const secret = process.env.CUCKOO_SECRET;
if (!secret) throw new Error("set CUCKOO_SECRET to the binding secret");

const agent = new Agent({
  secret,
  hub: process.env.CUCKOO_HUB ?? "http://localhost:8080",
});

agent.onMessage(async (message, conversation) => {
  console.log(`${message.sender.displayName} said: ${message.text || "(nothing)"}`);

  if (message.attachments.length === 0) {
    await conversation.send(`You said: ${message.text}`);
    return;
  }

  // Files arrive as ids; the bytes are fetched only if something wants them.
  // Downloading and sending them straight back is the smallest honest proof
  // that both directions work.
  const sent = [];
  for (const attachment of message.attachments) {
    const bytes = await agent.client.download(attachment.mediaId);
    console.log(`got ${attachment.fileName} (${attachment.kind}, ${bytes.length} bytes)`);
    sent.push(
      await agent.client.upload(attachment.fileName || "file", bytes, {
        contentType: attachment.mimeType,
        ...(attachment.durationMs ? { durationMs: attachment.durationMs } : {}),
        ...(attachment.waveform.length ? { waveform: attachment.waveform } : {}),
        audio: attachment.kind === "audio",
      }),
    );
  }
  await conversation.send(message.text ? `You sent: ${message.text}` : "", { attachments: sent });
});

agent.onJoin(async (conversation) => {
  console.log(`joined ${conversation.id}`);
  await conversation.send("Hello. Say anything and I will say it back.");
});

console.log("echo: connecting…");
await agent.serve();
