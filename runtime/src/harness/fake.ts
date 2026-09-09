/**
 * A harness that says what it was told to say.
 *
 * The suite runs against this, so the whole loop — streaming, tool calls,
 * what is written down afterwards — is tested with no model key, no network,
 * and the same answer every time. A test that needs a model tests the model.
 */

import type { Harness, HarnessEvent, RunInput, Tool } from "./harness.ts";

/** What the fake should do, in order. */
export type Script =
  | { say: string }
  | { call: string; args?: Record<string, unknown> }
  | { fail: string };

export class FakeHarness implements Harness {
  private readonly script: Script[];
  /** Every run it was asked to do, for a test to inspect. */
  readonly runs: RunInput[] = [];

  constructor(script: Script[] = [{ say: "hello" }]) {
    this.script = script;
  }

  async *run(input: RunInput): AsyncIterable<HarnessEvent> {
    this.runs.push(input);
    const tools = new Map(input.tools.map((t: Tool) => [t.name, t]));

    for (const step of this.script) {
      if ("fail" in step) throw new Error(step.fail);

      if ("say" in step) {
        // In pieces, because that is how the real one arrives and the code
        // downstream should never depend on getting it whole.
        for (const word of step.say.split(" ")) {
          yield { type: "text_delta", text: word + " " };
        }
        continue;
      }

      const args = step.args ?? {};
      yield { type: "tool_start", name: step.call, args };
      const tool = tools.get(step.call);
      const result = tool
        ? await tool.execute(args)
        : { error: `no tool called ${step.call}` };
      yield { type: "tool_result", name: step.call, result };
    }

    yield { type: "done", usage: { inputTokens: 0, outputTokens: 0 } };
  }
}
