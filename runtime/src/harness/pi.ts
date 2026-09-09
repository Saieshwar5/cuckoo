/**
 * pi behind the Harness interface.
 *
 * This is the only file in the runtime that knows pi exists. Everything above
 * it speaks in `HarnessEvent`, so when pi changes — it is young and moves
 * fast — the change is a day here rather than a week everywhere.
 *
 * What pi gives us that is worth the dependency: one API over every provider,
 * so "the ten best models" is a list rather than ten integrations, and a tool
 * loop whose events map onto Cuckoo's streaming almost one to one.
 */

import { Agent, type AgentTool } from "@earendil-works/pi-agent-core";
import {
  createModels,
  type AssistantMessage,
  type Api,
  type Message,
  type Model,
  type Models,
  type TSchema,
} from "@earendil-works/pi-ai";
import { builtinProviders } from "@earendil-works/pi-ai/providers/all";

import type { Harness, HarnessEvent, HarnessMessage, RunInput, Tool } from "./harness.ts";

export interface PiHarnessOptions {
  apiKey: string;
  /**
   * Which of pi's providers to ask. Its catalogue has Anthropic, Fireworks,
   * OpenAI, Groq, Google and a few dozen more; a model id is that provider's
   * own, so Fireworks wants `accounts/fireworks/models/…`.
   */
  provider: string;
  /** Swapped in tests; defaults to pi's whole catalogue. */
  models?: Models;
}

export class PiHarness implements Harness {
  private readonly models: Models;
  private readonly apiKey: string;
  private readonly provider: string;

  constructor(options: PiHarnessOptions) {
    this.apiKey = options.apiKey;
    this.provider = options.provider;
    if (options.models) {
      this.models = options.models;
    } else {
      // Every provider pi ships, so changing one is a setting rather than a
      // deploy. This is the whole reason pi is worth the dependency: "the ten
      // best models" (D68) becomes a list instead of ten integrations.
      const models = createModels();
      for (const provider of builtinProviders()) models.setProvider(provider);
      this.models = models;
    }
  }

  async *run(input: RunInput): AsyncIterable<HarnessEvent> {
    const model = this.models.getModel(this.provider, input.model);
    if (!model) throw new Error(`no model called ${input.model} at ${this.provider}`);

    // The last message is what we are answering; the rest is history. pi wants
    // them separately: history on the state, the new one passed to prompt().
    const history = input.messages.slice(0, -1);
    const asking = input.messages.at(-1);
    if (!asking) throw new Error("a run needs at least one message");

    const agent = new Agent({
      streamFn: this.models.streamSimple.bind(this.models),
      // Re-resolved per call, which is what a rotating key needs.
      getApiKey: () => this.apiKey,
    });
    agent.state.systemPrompt = input.system;
    agent.state.model = model;
    agent.state.tools = input.tools.map((tool) => toAgentTool(tool));
    agent.state.messages = history.map((message) => toPiMessage(message, model));

    const queue = new EventQueue();

    // Listeners are awaited by the loop, so this one does the least possible:
    // map the event, push it, return. Anything slower would throttle the model.
    const unsubscribe = agent.subscribe((event) => {
      switch (event.type) {
        case "message_update": {
          const inner = event.assistantMessageEvent;
          if (inner.type === "text_delta") queue.push({ type: "text_delta", text: inner.delta });
          return;
        }
        case "tool_execution_start":
          queue.push({
            type: "tool_start",
            name: event.toolName,
            args: (event.args ?? {}) as Record<string, unknown>,
          });
          return;
        case "tool_execution_end":
          queue.push({
            type: "tool_result",
            name: event.toolName,
            result: unwrapToolResult(event.result),
          });
          return;
        case "message_end": {
          const message = event.message as AssistantMessage;
          // A stream never rejects: a refused request, a bad key and an abort
          // all arrive as a finished message wearing a stop reason. Turning it
          // back into a thrown error is this adapter's job, so the job queue
          // above can retry the way it retries everything else.
          if (message.stopReason === "error") {
            queue.fail(new Error(message.errorMessage || "the model failed"));
          }
          return;
        }
        default:
          return;
      }
    });

    if (input.signal) input.signal.addEventListener("abort", () => agent.abort(), { once: true });

    const finished = agent
      .prompt(asking.content)
      .then(() => queue.close(usageOf(agent.state.messages)))
      .catch((error: unknown) => queue.fail(error));

    try {
      yield* queue.drain();
    } finally {
      unsubscribe();
      await finished;
    }
  }
}

/**
 * One of our tools as pi wants it.
 *
 * pi types `parameters` as a TypeBox schema, and a TypeBox schema is a plain
 * JSON Schema object at runtime — so ours passes through, and the cast is the
 * type system catching up with that rather than a claim about the value.
 */
function toAgentTool(tool: Tool): AgentTool {
  return {
    name: tool.name,
    label: tool.name,
    description: tool.description,
    parameters: tool.parameters as unknown as TSchema,
    async execute(_toolCallId, params) {
      // pi wants a throw on failure, not an error in the content, and turns
      // one into a tool result the model can read and recover from.
      const result = await tool.execute((params ?? {}) as Record<string, unknown>);
      return { content: [{ type: "text", text: JSON.stringify(result) }], details: {} };
    },
  };
}

/**
 * A remembered turn as a pi message.
 *
 * An assistant message carries fields that only a real reply has — which
 * provider answered, what it cost, why it stopped. Ours is a row of text read
 * back from Postgres, so those are filled with what is true of it: nothing was
 * spent replaying it, and it ended by being finished.
 */
function toPiMessage(message: HarnessMessage, model: Model<Api>): Message {
  if (message.role === "user") {
    return { role: "user", content: message.content, timestamp: Date.now() };
  }
  return {
    role: "assistant",
    content: [{ type: "text", text: message.content }],
    // The model that is about to answer, because a remembered turn has to
    // look like one this provider produced.
    api: model.api,
    provider: model.provider,
    model: model.id,
    usage: {
      input: 0,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason: "stop",
    timestamp: Date.now(),
  } as AssistantMessage;
}

/**
 * The tool's own answer, out of the envelope pi wraps it in.
 *
 * pi hands a tool result back as `{ content: [{ type: "text", text }], … }`
 * with our JSON re-encoded as a string inside it. Storing that shape would
 * mean every later reader — compaction, recall, a person looking at a row —
 * has to know pi's envelope. Unwrapping here keeps pi behind this file, which
 * is the point of the file.
 */
function unwrapToolResult(result: unknown): unknown {
  const wrapper = result as { content?: { type?: string; text?: string }[] } | null;
  const text = wrapper?.content?.find((part) => part?.type === "text")?.text;
  if (typeof text !== "string") return result;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

/** What the run cost, summed over the assistant messages it produced. */
function usageOf(messages: readonly unknown[]): { inputTokens: number; outputTokens: number } {
  let inputTokens = 0;
  let outputTokens = 0;
  for (const message of messages) {
    const candidate = message as { role?: string; usage?: { input?: number; output?: number } };
    if (candidate.role !== "assistant" || !candidate.usage) continue;
    inputTokens += candidate.usage.input ?? 0;
    outputTokens += candidate.usage.output ?? 0;
  }
  return { inputTokens, outputTokens };
}

/**
 * Turns pi's callbacks into something `for await` can read.
 *
 * pi tells you things by calling you; the rest of this runtime reads a stream.
 * This is the adapter between the two, and it is deliberately small: a list, a
 * waiter, and the three ways it can end.
 */
class EventQueue {
  private readonly events: HarnessEvent[] = [];
  private wake?: () => void;
  private done = false;
  private error?: unknown;

  push(event: HarnessEvent): void {
    this.events.push(event);
    this.wake?.();
  }

  close(usage: { inputTokens: number; outputTokens: number }): void {
    this.events.push({ type: "done", usage });
    this.done = true;
    this.wake?.();
  }

  fail(error: unknown): void {
    this.error = error;
    this.done = true;
    this.wake?.();
  }

  async *drain(): AsyncIterable<HarnessEvent> {
    for (;;) {
      while (this.events.length) {
        yield this.events.shift()!;
      }
      if (this.error) throw this.error;
      if (this.done) return;
      await new Promise<void>((resolve) => {
        this.wake = resolve;
      });
      this.wake = undefined;
    }
  }
}
