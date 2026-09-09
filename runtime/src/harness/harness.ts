/**
 * The seam between this runtime and whatever runs the model.
 *
 * Everything above this file speaks in these three types. pi is behind it
 * (`pi.ts`), a scripted stand-in is behind it in tests (`fake.ts`), and if pi
 * changes shape — it is young and moves fast — the change stops here. That is
 * the entire reason this interface exists rather than calling pi directly.
 */

/** A tool the model may call. The schema is JSON Schema, which every provider takes. */
export interface Tool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  /** What actually happens. Anything returned is shown to the model. */
  execute(args: Record<string, unknown>): Promise<unknown>;
}

/** One prior turn, as the model should see it. */
export interface HarnessMessage {
  role: "user" | "assistant";
  content: string;
}

export interface RunInput {
  /** Who the agent is, in words. The persona. */
  system: string;
  /** The window: recent turns, oldest first. Never the whole history. */
  messages: HarnessMessage[];
  tools: Tool[];
  model: string;
  /** Aborts the run when the person is no longer waiting for it. */
  signal?: AbortSignal;
}

/**
 * What a run emits as it happens.
 *
 * `text_delta` is forwarded to the hub as it arrives, which is what draws the
 * reply word by word. `tool_start` shows the typing indicator: a tool call is
 * the part where nothing appears for a second or two and the person needs to
 * know something is happening.
 */
export type HarnessEvent =
  | { type: "text_delta"; text: string }
  | { type: "tool_start"; name: string; args: Record<string, unknown> }
  | { type: "tool_result"; name: string; result: unknown }
  | { type: "done"; usage?: Usage };

export interface Usage {
  inputTokens: number;
  outputTokens: number;
}

export interface Harness {
  run(input: RunInput): AsyncIterable<HarnessEvent>;
}
