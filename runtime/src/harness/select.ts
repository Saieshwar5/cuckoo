/**
 * Which harness a process runs with.
 *
 * `RUNTIME_HARNESS=fake` runs the whole service — the door, the queue, the
 * streaming, the tools — with a scripted model instead of a real one. That is
 * how the loop is exercised against a live hub without a key and without
 * spending anything, and it is the difference between "the tests pass" and
 * "it works".
 */

import type { Config } from "../config.ts";
import { FakeHarness } from "./fake.ts";
import type { Harness } from "./harness.ts";
import { PiHarness } from "./pi.ts";

export function selectHarness(config: Config, env: NodeJS.ProcessEnv = process.env): Harness {
  if (env.RUNTIME_HARNESS === "fake") {
    console.log("runtime: using the scripted harness — no model will be called");
    // It calls the tool it is given and reports what came back, so a run
    // against a live hub still proves tools, streaming and typing.
    return new FakeHarness([
      { call: "weather", args: { place: env.RUNTIME_FAKE_PLACE ?? "Hyderabad", days: 2 } },
      { say: "That is what the forecast says." },
    ]);
  }
  if (!config.anthropicApiKey) {
    throw new Error("ANTHROPIC_API_KEY is not set (or set RUNTIME_HARNESS=fake)");
  }
  return new PiHarness({ apiKey: config.anthropicApiKey });
}
