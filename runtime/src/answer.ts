/**
 * Answering one message: the loop the whole runtime exists to run.
 *
 * Load who the agent is and what was said before, run the model with the
 * tools that agent may use, stream the reply to the hub as it is written,
 * then write down what happened and forget everything else. Nothing about a
 * run survives in memory — which is what lets one process answer for every
 * agent, and what lets it be restarted in the middle of a busy afternoon.
 */

import { StoppedError, type ActivityState, type Buttons, type HubClient } from "@cuckoo/agent";

import type { AgentRecord } from "./agents/registry.ts";
import {
  asksToConfirm,
  type Confirmation,
  type Harness,
  type HarnessMessage,
  type Tool,
  type ToolContext,
  type ToolFactory,
} from "./harness/harness.ts";
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

/** An activity lasts ten seconds on the hub; it is said again well before. */
const RENEW_MS = 7_000;

export interface AnswerInput {
  agent: AgentRecord;
  conversationId: string;
  /** What the person said, already parsed out of the event. */
  text: string;
  hubMessageId: string;
  /** Whose bill this run is. The hub's usr_… identifier. */
  userId: string;
  /** The hub schedule this run is for, when a routine fired it. */
  scheduleId?: string;
  /** The person's time zone, as their phone last told the hub. */
  timezone?: string;
}

export interface AnswerDeps {
  turns: TurnStore;
  harness: Harness;
  /**
   * The tools this runtime knows, by name, built per run: a tool that touches
   * a person's own things is given the conversation it was called in and
   * cannot reach outside it.
   */
  tools: Map<string, ToolFactory>;
  /** A client already authenticated as the agent being answered for. */
  client: HubClient;
  /** The ceiling, and the counter a subscription will one day read. */
  usage?: UsageLimit;
  windowSize?: number;
  /** Fires when the person presses stop: the model call ends with it. */
  signal?: AbortSignal;
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

  const tools = toolsFor(agent, deps.tools, {
    agentId: agent.id,
    conversationId,
    userId: input.userId,
    timezone: input.timezone,
  });
  // What the reply will end with, if a tool asked for a tap.
  let confirmation: Confirmation | undefined;
  const writer = new Writer(deps.client, conversationId, input.scheduleId);
  const labels = new Map(tools.map((t) => [t.name, t.activity]));

  // The model may think for a while before it says anything, and a tool call
  // is silence too. The indicator is what says the agent is not simply broken.
  await writer.busy("thinking");

  try {
    const events = deps.harness.run({
      system: withClock(agent.persona, input.timezone),
      messages: [...asMessages(history), { role: "user", content: input.text }],
      tools,
      model: agent.model,
      signal: deps.signal,
    });

    for await (const event of events) {
      if (deps.signal?.aborted) break;
      switch (event.type) {
        case "text_delta":
          await writer.write(event.text);
          break;
        case "tool_start": {
          // The person is told what, in the tool's own words, rather than
          // just that something is happening.
          const label = labels.get(event.name);
          await writer.busy(label ? "working" : "thinking", label);
          break;
        }
        case "tool_result":
          // A tool that would change something returns an offer instead of
          // doing it. The buttons go on the end of the reply; the tap is
          // carried out later by code, without asking the model again.
          if (asksToConfirm(event.result)) confirmation = event.result.confirm;
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

    if (deps.signal?.aborted) {
      await stoppedHere(input, deps, writer);
      return;
    }
    const said = await writer.close(confirmation);
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
    // The person pressed stop — here, or on another process, which the hub
    // reports on our next write. Not a failure: nothing to retry.
    if (deps.signal?.aborted || error instanceof StoppedError) {
      await stoppedHere(input, deps, writer);
      return;
    }
    // Whatever went wrong, the person is sitting in front of a chat that says
    // nothing. Close what was opened and say so in the one place they are
    // looking; the job's own retry decides whether to try again.
    await writer.fail();
    throw error;
  }
}

/**
 * The person pressed stop. The hub has already ended the reply and cleared
 * the indicator; what is left is to remember what was said before the stop,
 * so the next turn knows it was cut short rather than finished.
 */
async function stoppedHere(input: AnswerInput, deps: AnswerDeps, writer: Writer): Promise<void> {
  const said = writer.abandon();
  await deps.turns.record({
    agentId: input.agent.id,
    conversationId: input.conversationId,
    role: "assistant",
    text: `${said.text.trim()}${said.text.trim() ? " " : ""}[the person stopped this reply]`,
    hubMessageId: said.messageId,
  });
}

/**
 * The persona, with the person's clock at the end of it.
 *
 * A model has no idea what time it is or where the person is standing. "In
 * two hours", "tomorrow at 7" and "this weekend" all need both, and a model
 * told neither assumes whatever its training leaned towards.
 */
export function withClock(persona: string, timezone: string | undefined, now = new Date()): string {
  if (!timezone) return persona;
  let local: string;
  try {
    local = new Intl.DateTimeFormat("en-GB", {
      timeZone: timezone,
      weekday: "long",
      day: "numeric",
      month: "long",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).format(now);
  } catch {
    return persona;
  }
  return `${persona}\n\nThe person's time zone is ${timezone}. It is ${local} there now. ` +
    `Use their time zone for any time they mention, unless they name another.`;
}

/** The tools an agent may use: what its template names, and nothing else. */
function toolsFor(
  agent: AgentRecord,
  known: Map<string, ToolFactory>,
  context: ToolContext,
): Tool[] {
  const tools: Tool[] = [];
  for (const name of agent.template.tools) {
    const build = known.get(name);
    if (build) tools.push(build(context));
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
 * Writes the reply to the hub: keeps the person told what the agent is doing,
 * opens the stream when there is finally something to say, and buffers so a
 * token does not become a request.
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
  private renew?: ReturnType<typeof setInterval>;
  // Whether an activity is up on the person's screen right now.
  private showing = false;
  // The schedule the reply is for, so the person sees why it came.
  private readonly scheduleId?: string;

  constructor(client: HubClient, conversationId: string, scheduleId?: string) {
    this.client = client;
    this.conversationId = conversationId;
    this.scheduleId = scheduleId;
  }

  /**
   * Say what is happening, and keep saying it: a model can think for longer
   * than an activity lasts. Safe to call repeatedly.
   */
  async busy(state: Exclude<ActivityState, "idle">, label?: string): Promise<void> {
    // Once a reply is open, only a named tool is worth saying over it: a
    // model often writes a line, then stops to look something up, and the
    // person should see what, not a reply that has gone quiet.
    if (this.stream && !label) return;
    const say = () => this.client.activity(this.conversationId, state, label).catch(() => {});
    if (this.renew) clearInterval(this.renew);
    this.renew = setInterval(() => void say(), RENEW_MS);
    this.showing = true;
    await say();
  }

  private quiet(): void {
    if (this.renew) clearInterval(this.renew);
    this.renew = undefined;
    this.showing = false;
  }

  private async idle(): Promise<void> {
    this.quiet();
    await this.client.activity(this.conversationId, "idle").catch(() => {});
  }

  async write(text: string): Promise<void> {
    if (!this.stream) {
      // Blank space is not something to say. Models often send a line break
      // or two before calling a tool; opening a reply for it would put an
      // empty bubble on screen and hide what the agent is actually doing.
      text = text.replace(/^\s+/, "");
    }
    if (!text) return;
    // Words again after a tool: its name comes down, and "writing…" is true.
    if (this.stream && this.showing) await this.idle();
    this.text += text;
    this.buffer += text;
    if (!this.stream) {
      this.quiet();
      const started = await this.client.startStream(this.conversationId, { scheduleId: this.scheduleId });
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
  async close(confirmation?: Confirmation): Promise<{ messageId: string; text: string } | undefined> {
    if (!this.stream) {
      await this.idle();
      // A tool asked for a tap and the model said nothing at all. The question
      // still has to reach the person, or the offer is lost.
      if (confirmation) {
        const sent = await this.client.send(this.conversationId, "Shall I?", {
          buttons: buttonsFor(confirmation),
        });
        return { messageId: sent.id, text: "Shall I?" };
      }
      return undefined;
    }
    await this.flush();
    await this.stream.finish(confirmation ? { buttons: buttonsFor(confirmation) } : {});
    return { messageId: this.stream.messageId, text: this.text };
  }

  /**
   * Let go after a stop, sending nothing: the hub has ended the reply where
   * the person saw it and cleared the indicator. Returns what reached them.
   */
  abandon(): { messageId: string; text: string } {
    this.quiet();
    // What was still in the buffer never reached the screen.
    const shown = this.text.slice(0, this.text.length - this.buffer.length);
    this.buffer = "";
    return { messageId: this.stream?.messageId ?? "", text: shown };
  }

  /** Close down after a failure, leaving nothing half-open. */
  async fail(): Promise<void> {
    try {
      if (this.stream) {
        await this.flush().catch(() => {});
        await this.stream.finish().catch(() => {});
      } else {
        await this.idle();
      }
    } catch {
      // The hub being unreachable is what put us here; nothing more to do.
    }
  }
}

/** Yes and no, in that order. Buttons are attached when a stream finishes. */
function buttonsFor(confirmation: Confirmation): Buttons {
  return [
    [
      { id: confirmation.buttonId, label: confirmation.label },
      { id: `${confirmation.buttonId}_no`, label: confirmation.cancelLabel ?? "Cancel" },
    ],
  ];
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

  async finish(options: { buttons?: Buttons } = {}): Promise<void> {
    if (this.finished) return;
    this.finished = true;
    await this.client.finishStream(this.messageId, this.conversationId, options);
  }
}
