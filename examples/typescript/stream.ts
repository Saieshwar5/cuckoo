/**
 * A reply that arrives word by word, with buttons at the end.
 *
 * What the app draws for this is a bubble with a caret that fills as the text
 * comes in — the thing chat apps built for people cannot do for a bot.
 *
 *   export CUCKOO_SECRET=bnd_sec_...
 *   npm run stream
 */

import { Agent } from "@cuckoo/agent";

const secret = process.env.CUCKOO_SECRET;
if (!secret) throw new Error("set CUCKOO_SECRET to the binding secret");

const agent = new Agent({ secret, hub: process.env.CUCKOO_HUB ?? "http://localhost:8080" });

const REPLY =
  "Streaming means the person sees the answer being written instead of " +
  "waiting for it. Every word here was a separate append, and the hub " +
  "delivered each one as it arrived.";

agent.onMessage(async (message, conversation) => {
  // A tap on one of the buttons below comes back as a message whose action
  // names the button; the text is its label.
  if (message.action) {
    await conversation.send(`You tapped ${message.action.buttonId}.`);
    return;
  }

  const reply = await conversation.stream();
  for (const word of REPLY.split(" ")) {
    await reply.append(`${word} `);
    await new Promise((resolve) => setTimeout(resolve, 60));
  }
  // Buttons are attached at the end, never at the start.
  await reply.finish({
    buttons: [[{ id: "again", label: "Again" }, { id: "stop", label: "That's enough" }]],
  });
});

console.log("stream: connecting…");
await agent.serve();
