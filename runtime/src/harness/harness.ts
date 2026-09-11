/**
 * The seam between this runtime and whatever runs the model.
 *
 * Everything above this file speaks in these three types. pi is behind it
 * (`pi.ts`), a scripted stand-in is behind it in tests (`fake.ts`), and if pi
 * changes shape — it is young and moves fast — the change stops here. That is
 * the entire reason this interface exists rather than calling pi directly.
 */

/**
 * What a tool may ask for instead of acting: a button the person must tap.
 *
 * A tool returns this when what it would do changes the world. The loop
 * attaches the buttons to the reply; the tap is looked up and carried out by
 * code that does not consult the model again. The model chooses the words, not
 * whether there is a button.
 */
export interface Confirmation {
  /** The id recorded against the pending action; it comes back on the tap. */
  buttonId: string;
  label: string;
  cancelLabel?: string;
}

/** A tool result that wants a tap before anything happens. */
export interface ConfirmingResult {
  confirm: Confirmation;
  [key: string]: unknown;
}

export function asksToConfirm(result: unknown): result is ConfirmingResult {
  const candidate = result as { confirm?: { buttonId?: unknown } } | null;
  return typeof candidate?.confirm?.buttonId === "string";
}

/** A tool the model may call. The schema is JSON Schema, which every provider takes. */
export interface Tool {
  name: string;
  description: string;
  /**
   * What the person sees while it runs, under the agent's name: "Checking
   * the weather". One line, at most 40 characters, no links. Without one
   * they see "thinking…".
   */
  activity?: string;
  parameters: Record<string, unknown>;
  /** What actually happens. Anything returned is shown to the model. */
  execute(args: Record<string, unknown>): Promise<unknown>;
}

/** One prior turn, as the model should see it. */
export interface HarnessMessage {
  role: "user" | "assistant";
  content: string;
}

/**
 * Who a run is for. Tools that touch a person's own things — their routines,
 * later their mail — are built per run with this, so a tool cannot reach
 * outside the conversation it was called in even if the model asks it to.
 */
export interface ToolContext {
  agentId: string;
  conversationId: string;
  userId: string;
  /** Where the person's phone's clock is, when the hub knows: "Europe/London". */
  timezone?: string;
}

/** A tool that needs to know whose conversation it is in. */
export type ToolFactory = (context: ToolContext) => Tool;

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
