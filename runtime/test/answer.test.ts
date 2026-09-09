/**
 * The loop, end to end, with a scripted model and a stub hub.
 *
 * This is the test that would catch the runtime answering the wrong person,
 * forgetting what was said, or opening a stream it never closes — the three
 * ways this service can be quietly, expensively wrong.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import type { HubClient } from "@cuckoo/agent";

import { answer } from "../src/answer.ts";
import type { AgentRecord } from "../src/agents/registry.ts";
import { FakeHarness, type Script } from "../src/harness/fake.ts";
import type { Tool } from "../src/harness/harness.ts";
import type { RecordInput, Turn, TurnStore } from "../src/memory/turns.ts";

/** A hub that records what it was told, and never fails. */
class StubHub {
  readonly calls: string[] = [];
  readonly appended: string[] = [];
  private streams = 0;

  async typing(_conversationId: string, state: string): Promise<void> {
    this.calls.push(`typing:${state}`);
  }
  async startStream(_conversationId: string): Promise<{ id: string }> {
    this.streams += 1;
    this.calls.push("stream.start");
    return { id: `msg_stream_${this.streams}` };
  }
  async appendStream(_messageId: string, text: string): Promise<void> {
    this.calls.push("stream.append");
    this.appended.push(text);
  }
  async finishStream(messageId: string): Promise<{ id: string }> {
    this.calls.push("stream.finish");
    return { id: messageId };
  }
  said(): string {
    return this.appended.join("");
  }
  asClient(): HubClient {
    return this as unknown as HubClient;
  }
}

class MemoryTurns implements TurnStore {
  readonly rows: RecordInput[] = [];
  history: Turn[] = [];

  async record(input: RecordInput): Promise<void> {
    this.rows.push(input);
  }
  async window(): Promise<Turn[]> {
    return this.history;
  }
}

const AGENT: AgentRecord = {
  id: "00000000-0000-0000-0000-000000000001",
  hubAgentId: "agt_1",
  ownerUserId: "usr_cuckoo",
  template: { id: "weather", name: "Weather", persona: "You are the weather.", tools: ["weather"], model: "", starters: [] },
  persona: "You are the weather.",
  model: "claude-sonnet-5",
  private: false,
};

function setup(script: Script[], tools: Tool[] = []) {
  const hub = new StubHub();
  const turns = new MemoryTurns();
  const harness = new FakeHarness(script);
  const deps = {
    turns,
    harness,
    tools: new Map(tools.map((t) => [t.name, t])),
    client: hub.asClient(),
  };
  return { hub, turns, harness, deps };
}

const ASKED = { agent: AGENT, conversationId: "cnv_1", text: "will it rain?", hubMessageId: "msg_1" };

test("answers, streams the words, and writes down both sides", async () => {
  const { hub, turns, deps } = setup([{ say: "yes, tomorrow" }]);

  await answer(ASKED, deps);

  assert.equal(hub.said().trim(), "yes, tomorrow");
  // Typing first, because the model is silent for a moment; then one stream.
  assert.equal(hub.calls[0], "typing:start");
  assert.equal(hub.calls.filter((c) => c === "stream.start").length, 1);
  assert.equal(hub.calls.at(-1), "stream.finish");

  const roles = turns.rows.map((r) => r.role);
  assert.deepEqual(roles, ["user", "assistant"]);
  assert.equal(turns.rows[0]!.text, "will it rain?");
  assert.equal(turns.rows[0]!.hubMessageId, "msg_1");
  assert.equal(turns.rows[1]!.text!.trim(), "yes, tomorrow");
});

test("the model is given the persona and the history, then the new message", async () => {
  const { harness, turns, deps } = setup([{ say: "ok" }]);
  turns.history = [
    { id: "1", role: "user", text: "hello", createdAt: "" },
    { id: "2", role: "assistant", text: "hi", createdAt: "" },
    // A tool row is remembered but is not replayed to the model: the answer
    // it produced is already in the transcript.
    { id: "3", role: "tool", text: "", toolName: "weather", createdAt: "" },
  ];

  await answer(ASKED, deps);

  const run = harness.runs[0]!;
  assert.equal(run.system, "You are the weather.");
  assert.equal(run.model, "claude-sonnet-5");
  assert.deepEqual(run.messages, [
    { role: "user", content: "hello" },
    { role: "assistant", content: "hi" },
    { role: "user", content: "will it rain?" },
  ]);
});

test("an agent is offered only the tools its template names", async () => {
  const weather: Tool = {
    name: "weather",
    description: "",
    parameters: {},
    execute: async () => ({ ok: true }),
  };
  const mail: Tool = { name: "mail", description: "", parameters: {}, execute: async () => ({}) };
  const { harness, deps } = setup([{ say: "ok" }], [weather, mail]);

  await answer(ASKED, deps);

  assert.deepEqual(harness.runs[0]!.tools.map((t) => t.name), ["weather"]);
});

test("a tool call shows the indicator and its result is remembered", async () => {
  const weather: Tool = {
    name: "weather",
    description: "",
    parameters: {},
    execute: async () => ({ highC: 31, rainChancePercent: 80 }),
  };
  const { hub, turns, deps } = setup(
    [{ call: "weather", args: { place: "Hyderabad" } }, { say: "80% chance" }],
    [weather],
  );

  await answer(ASKED, deps);

  // The indicator goes up again while the tool runs: that is the pause the
  // person would otherwise read as nothing happening.
  assert.equal(hub.calls.filter((c) => c === "typing:start").length, 2);
  const toolRow = turns.rows.find((r) => r.role === "tool");
  assert.equal(toolRow?.toolName, "weather");
  assert.deepEqual(toolRow?.toolResult, { highC: 31, rainChancePercent: 80 });
  assert.equal(hub.said().trim(), "80% chance");
});

test("a stream is only opened when there is something to say", async () => {
  // A run that calls a tool and then says nothing must not leave an empty
  // bubble on the person's screen, or a stream for the hub to time out.
  const tool: Tool = { name: "weather", description: "", parameters: {}, execute: async () => ({}) };
  const { hub, turns, deps } = setup([{ call: "weather" }], [tool]);

  await answer(ASKED, deps);

  assert.equal(hub.calls.includes("stream.start"), false);
  assert.equal(hub.calls.at(-1), "typing:stop");
  assert.equal(turns.rows.some((r) => r.role === "assistant"), false);
});

test("a model that fails leaves nothing open, and the failure is passed up", async () => {
  const { hub, deps } = setup([{ fail: "the provider is down" }]);

  await assert.rejects(() => answer(ASKED, deps), /the provider is down/);

  // The job queue decides whether to retry; what matters here is that the
  // indicator was taken down rather than left spinning forever.
  assert.equal(hub.calls.at(-1), "typing:stop");
});
