/**
 * Answering one message: the loop the whole runtime exists to run.
 *
 * Load who the agent is and what was said before, run the model with the
 * tools that agent may use, stream the reply to the hub as it is written,
 * then write down what happened and forget everything else. Nothing about a
 * run survives in memory — which is what lets one process answer for every
 * agent, and what lets it be restarted in the middle of a busy afternoon.
 */

import type { HubClient } from "@cuckoo/agent";

import type { AgentRecord } from "./agents/registry.ts";
import type { Harness, HarnessMessage, Tool } from "./harness/harness.ts";
import type { Turn, TurnStore } from "./memory/turns.ts";

/**
 * How long text is allowed to pool before it is sent on.
 *
 * A token at a time would be one HTTP call per word. Whole sentences at a
 * time would lose the point of streaming. A quarter second is under what a
 * person reads as a pause.
 */
const FLUSH_MS = 250;
const FLUSH_CHARS = 120;

export interface AnswerInput {
  agent: AgentRecord;
  conversationId: string;
  /** What the person said, already parsed out of the event. */
  text: string;
  hubMessageId: string;
  /** Whose bill this run is. The hub's usr_… identifier. */
  userId: string;
}

export interface AnswerDeps {
  turns: TurnStore;
  harness: Harness;
  /** The tools this runtime knows, by name. An agent gets the ones it names. */
  tools: Map<string, Tool>;
  /** A client already authenticated as the agent being answered for. */
  client: HubClient;
  /** The ceiling, and the counter a subscription will one day read. */
  usage?: UsageLimit;
  windowSize?: number;
}

/** What `answer` needs of the usage store, so a test can stand one in. */
export interface UsageLimit {
  withinLimit(userId: string): Promise<boolean>;
  record(
    userId: string,
    agentId: string,
    tokens: { inputTokens: number; outputTokens: number },
  ): Promise<void>;
}

export async function answer(input: AnswerInput, deps: AnswerDeps): Promise<void> {
  const { agent, conversationId } = input;

  // Asked before anything is loaded or spent. A run refused here costs
  // nothing; one cut off halfway has already been paid for and leaves half a
  // sentence on somebody's screen.
  if (deps.usage && !(await deps.usage.withinLimit(input.userId))) {
    await deps.client
      .send(
        conversationId,
        "You have reached today's limit for this agent. It will work again tomorrow.",
      )
      .catch(() => {});
    return;
  }

  // The window before this message is recorded, so the model is not handed
  // the thing it is about to be asked as though it were history.
  const history = await deps.turns.window(agent.id, conversationId, deps.windowSize);
  await deps.turns.record({
    agentId: agent.id,
    conversationId,
    role: "user",
    text: input.text,
    hubMessageId: input.hubMessageId,
  });

  const tools = toolsFor(agent, deps.tools);
  const writer = new Writer(deps.client, conversationId);

  // The model may think for a while before it says anything, and a tool call
  // is silence too. The indicator is what says the agent is not simply broken.
  await writer.thinking();

  try {
    const events = deps.harness.run({
      system: agent.persona,
      messages: [...asMessages(history), { role: "user", content: input.text }],
      tools,
      model: agent.model,
    });

    for await (const event of events) {
      switch (event.type) {
        case "text_delta":
          await writer.write(event.text);
          break;
        case "tool_start":
          await writer.thinking();
          break;
        case "tool_result":
          await deps.turns.record({
            agentId: agent.id,
            conversationId,
            role: "tool",
            toolName: event.name,
            toolResult: event.result,
          });
          break;
        case "done":
          if (deps.usage && event.usage) {
            await deps.usage.record(input.userId, agent.id, event.usage);
          }
          break;
      }
    }

    const said = await writer.close();
    if (said) {
      await deps.turns.record({
        agentId: agent.id,
        conversationId,
        role: "assistant",
        text: said.text,
        hubMessageId: said.messageId,
      });
    }
  } catch (error) {
    // Whatever went wrong, the person is sitting in front of a chat that says
    // nothing. Close what was opened and say so in the one place they are
    // looking; the job's own retry decides whether to try again.
    await writer.fail();
    throw error;
  }
}

/** The tools an agent may use: what its template names, and nothing else. */
function toolsFor(agent: AgentRecord, known: Map<string, Tool>): Tool[] {
  const tools: Tool[] = [];
  for (const name of agent.template.tools) {
    const tool = known.get(name);
    if (tool) tools.push(tool);
  }
  return tools;
}

/** How much of a remembered tool result the model is shown again. */
const TOOL_RECALL_CHARS = 400;

/**
 * The window as the model wants it, tool calls included.
 *
 * Leaving tool rows out was the first design here, on the reasoning that what
 * a tool returned had already been folded into the answer beside it. Running
 * a real model against a real conversation showed what that actually teaches:
 * a transcript where the assistant produces facts from nowhere, again and
 * again, is a demonstration that facts do not need looking up. After a few
 * turns the weather agent stopped calling the weather tool and began inventing
 * plausible temperatures instead — which is worse than refusing, because it
 * looks exactly like working.
 *
 * So the calls stay in, with their results trimmed. The model is shown a
 * conversation in which every answer was preceded by a lookup, because that is
 * what happened, and it is what should happen again.
 */
function asMessages(history: Turn[]): HarnessMessage[] {
  const out: HarnessMessage[] = [];
  for (const turn of history) {
    if (turn.role === "tool") {
      const result = JSON.stringify(turn.toolResult ?? {});
      out.push({
        role: "assistant",
        content:
          `[called ${turn.toolName ?? "a tool"} and it returned ` +
          `${result.slice(0, TOOL_RECALL_CHARS)}${result.length > TOOL_RECALL_CHARS ? "…" : ""}]`,
      });
      continue;
    }
    if (!turn.text) continue;
    out.push({
      role: turn.role === "assistant" ? "assistant" : "user",
      content: turn.text,
    });
  }
  return out;
}

/**
 * Writes the reply to the hub: holds the typing indicator, opens the stream
 * when there is finally something to say, and buffers so a token does not
 * become a request.
 *
 * The stream is opened lazily on purpose. The hub finishes a stream left idle
 * for thirty seconds and marks it truncated, so opening one before the model
 * has produced a word means racing a clock for no reason.
 */
class Writer {
  private readonly client: HubClient;
  private readonly conversationId: string;
  private stream?: StreamHandle;
  private buffer = "";
  private lastFlush = 0;
  private text = "";

  constructor(client: HubClient, conversationId: string) {
    this.client = client;
    this.conversationId = conversationId;
  }

  /** Say that something is happening. Safe to call repeatedly. */
  async thinking(): Promise<void> {
    if (this.stream) return; // a stream shows the indicator by itself
    await this.client.typing(this.conversationId, "start").catch(() => {});
  }

  async write(text: string): Promise<void> {
    if (!text) return;
    this.text += text;
    this.buffer += text;
    if (!this.stream) {
      const started = await this.client.startStream(this.conversationId);
      this.stream = new StreamHandle(this.client, this.conversationId, started.id);
      this.lastFlush = Date.now();
    }
    const due = Date.now() - this.lastFlush >= FLUSH_MS || this.buffer.length >= FLUSH_CHARS;
    if (due) await this.flush();
  }

  private async flush(): Promise<void> {
    if (!this.stream || !this.buffer) return;
    const pending = this.buffer;
    this.buffer = "";
    this.lastFlush = Date.now();
    await this.stream.append(pending);
  }

  /** Finish the message. Returns what was said, or undefined if nothing was. */
  async close(): Promise<{ messageId: string; text: string } | undefined> {
    if (!this.stream) {
      await this.client.typing(this.conversationId, "stop").catch(() => {});
      return undefined;
    }
    await this.flush();
    await this.stream.finish();
    return { messageId: this.stream.messageId, text: this.text };
  }

  /** Close down after a failure, leaving nothing half-open. */
  async fail(): Promise<void> {
    try {
      if (this.stream) {
        await this.flush().catch(() => {});
        await this.stream.finish().catch(() => {});
      } else {
        await this.client.typing(this.conversationId, "stop").catch(() => {});
      }
    } catch {
      // The hub being unreachable is what put us here; nothing more to do.
    }
  }
}

/** The SDK's Stream, without importing its class for a type-only use. */
class StreamHandle {
  private readonly client: HubClient;
  private readonly conversationId: string;
  readonly messageId: string;
  private finished = false;

  constructor(client: HubClient, conversationId: string, messageId: string) {
    this.client = client;
    this.conversationId = conversationId;
    this.messageId = messageId;
  }

  async append(text: string): Promise<void> {
    await this.client.appendStream(this.messageId, text);
  }

  async finish(): Promise<void> {
    if (this.finished) return;
    this.finished = true;
    await this.client.finishStream(this.messageId, this.conversationId);
  }
}
