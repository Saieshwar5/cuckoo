/**
 * The loop, end to end, with a scripted model and a stub hub.
 *
 * This is the test that would catch the runtime answering the wrong person,
 * forgetting what was said, or opening a stream it never closes — the three
 * ways this service can be quietly, expensively wrong.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { StoppedError, type HubClient } from "@cuckoo/agent";

import { answer, withClock } from "../src/answer.ts";
import type { AgentRecord } from "../src/agents/registry.ts";
import { FakeHarness, type Script } from "../src/harness/fake.ts";
import type { Tool } from "../src/harness/harness.ts";
import type { RecordInput, Turn, TurnStore } from "../src/memory/turns.ts";

/** A hub that records what it was told, and never fails. */
class StubHub {
  readonly calls: string[] = [];
  readonly appended: string[] = [];
  private streams = 0;
  /** Refuse appends as the hub does once the person has pressed stop. */
  stopped = false;

  async send(_conversationId: string, text: string): Promise<{ id: string }> {
    this.calls.push("send");
    this.appended.push(text);
    return { id: "msg_sent" };
  }
  async activity(_conversationId: string, state: string, label?: string): Promise<void> {
    this.calls.push(label ? `activity:${state}:${label}` : `activity:${state}`);
  }
  async startStream(_conversationId: string): Promise<{ id: string }> {
    this.streams += 1;
    this.calls.push("stream.start");
    return { id: `msg_stream_${this.streams}` };
  }
  async appendStream(_messageId: string, text: string): Promise<void> {
    if (this.stopped) throw new StoppedError();
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
    tools: new Map(tools.map((t) => [t.name, () => t])),
    client: hub.asClient(),
  };
  return { hub, turns, harness, deps };
}

const ASKED = {
  agent: AGENT,
  conversationId: "cnv_1",
  text: "will it rain?",
  hubMessageId: "msg_1",
  userId: "usr_priya",
};

test("answers, streams the words, and writes down both sides", async () => {
  const { hub, turns, deps } = setup([{ say: "yes, tomorrow" }]);

  await answer(ASKED, deps);

  assert.equal(hub.said().trim(), "yes, tomorrow");
  // Thinking first, because the model is silent for a moment; then one stream.
  assert.equal(hub.calls[0], "activity:thinking");
  assert.equal(hub.calls.filter((c) => c === "stream.start").length, 1);
  assert.equal(hub.calls.at(-1), "stream.finish");

  const roles = turns.rows.map((r) => r.role);
  assert.deepEqual(roles, ["user", "assistant"]);
  assert.equal(turns.rows[0]!.text, "will it rain?");
  assert.equal(turns.rows[0]!.hubMessageId, "msg_1");
  assert.equal(turns.rows[1]!.text!.trim(), "yes, tomorrow");
});

test("the window keeps tool calls, so the model is shown that answers were looked up", async () => {
  const { harness, turns, deps } = setup([{ say: "ok" }]);
  turns.history = [
    { id: "1", role: "user", text: "hello", createdAt: "" },
    { id: "2", role: "assistant", text: "hi", createdAt: "" },
    { id: "3", role: "tool", text: "", toolName: "weather", toolResult: { highC: 31 }, createdAt: "" },
  ];

  await answer(ASKED, deps);

  const run = harness.runs[0]!;
  assert.equal(run.system, "You are the weather.");
  assert.equal(run.model, "claude-sonnet-5");
  // The tool call stays in. Leaving it out was the first design, and a real
  // model against a real conversation showed what that teaches: a transcript
  // where the assistant produces facts from nowhere is a demonstration that
  // facts need no looking up. The weather agent stopped calling the weather
  // tool and began inventing temperatures, which looks exactly like working.
  assert.deepEqual(run.messages, [
    { role: "user", content: "hello" },
    { role: "assistant", content: "hi" },
    { role: "assistant", content: '[called weather and it returned {"highC":31}]' },
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
  assert.equal(hub.calls.filter((c) => c === "activity:thinking").length, 2);
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
  assert.equal(hub.calls.at(-1), "activity:idle");
  assert.equal(turns.rows.some((r) => r.role === "assistant"), false);
});

test("a model that fails leaves nothing open, and the failure is passed up", async () => {
  const { hub, deps } = setup([{ fail: "the provider is down" }]);

  await assert.rejects(() => answer(ASKED, deps), /the provider is down/);

  // The job queue decides whether to retry; what matters here is that the
  // indicator was taken down rather than left spinning forever.
  assert.equal(hub.calls.at(-1), "activity:idle");
});

test("a tool that names what it does is what the person sees while it runs", async () => {
  const weather: Tool = {
    name: "weather",
    activity: "Checking the weather",
    description: "",
    parameters: {},
    execute: async () => ({ highC: 31 }),
  };
  const { hub, deps } = setup([{ call: "weather" }, { say: "31 degrees" }], [weather]);

  await answer(ASKED, deps);

  assert.deepEqual(hub.calls.slice(0, 2), ["activity:thinking", "activity:working:Checking the weather"]);
});

test("blank space before a tool call opens no reply, so the person sees what the tool is doing", async () => {
  const weather: Tool = {
    name: "weather",
    activity: "Checking the weather",
    description: "",
    parameters: {},
    execute: async () => ({ highC: 31 }),
  };
  const { hub, deps } = setup([{ say: "\n\n" }, { call: "weather" }, { say: "31 degrees" }], [weather]);

  await answer(ASKED, deps);

  const working = hub.calls.indexOf("activity:working:Checking the weather");
  assert.ok(working > 0, "the tool's label was shown");
  assert.ok(hub.calls.indexOf("stream.start") > working, "the reply opened only once there were words");
  assert.equal(hub.said(), "31 degrees ");
});

test("a tool called mid-reply is still named, and the name comes down when words resume", async () => {
  const weather: Tool = {
    name: "weather",
    activity: "Checking the weather",
    description: "",
    parameters: {},
    execute: async () => ({ highC: 31 }),
  };
  const { hub, deps } = setup([{ say: "I'll look." }, { call: "weather" }, { say: "31 degrees" }], [weather]);

  await answer(ASKED, deps);

  const opened = hub.calls.indexOf("stream.start");
  const named = hub.calls.indexOf("activity:working:Checking the weather");
  assert.ok(opened >= 0 && named > opened, "the tool is named although a reply is open");
  assert.equal(hub.calls[named + 1], "activity:idle", "and taken down when the words come back");
});

test("a stop ends the run where it stands: nothing more is said, and the cut is remembered", async () => {
  const stop = new AbortController();
  // The stop lands while a tool runs, after the first words were written.
  const slow: Tool = {
    name: "weather",
    description: "",
    parameters: {},
    execute: async () => {
      stop.abort();
      return {};
    },
  };
  const { hub, turns, deps } = setup(
    [{ say: "Let me look." }, { call: "weather" }, { say: "It will never say this" }],
    [slow],
  );

  await answer(ASKED, { ...deps, signal: stop.signal });

  assert.equal(hub.said().includes("never"), false);
  // The hub ended the reply and cleared the indicator itself.
  assert.equal(hub.calls.includes("stream.finish"), false);
  assert.equal(hub.calls.at(-1) === "activity:idle", false);
  const last = turns.rows.at(-1)!;
  assert.equal(last.role, "assistant");
  assert.match(last.text ?? "", /\[the person stopped this reply\]$/);
});

test("a stop felt only at the hub ends the run quietly rather than as a failure to retry", async () => {
  const long = "word ".repeat(40); // past the flush size, so the words are sent
  const { hub, turns, deps } = setup([{ say: long }]);
  hub.stopped = true;

  await answer(ASKED, deps); // resolves: the job is done, not retried

  assert.match(turns.rows.at(-1)?.text ?? "", /\[the person stopped this reply\]$/);
});

/** A ceiling a test can set, and a ledger it can read. */
class Ledger {
  allow = true;
  readonly recorded: { userId: string; agentId: string; inputTokens: number; outputTokens: number }[] = [];

  async withinLimit(): Promise<boolean> {
    return this.allow;
  }
  async record(
    userId: string,
    agentId: string,
    tokens: { inputTokens: number; outputTokens: number },
  ): Promise<void> {
    this.recorded.push({ userId, agentId, ...tokens });
  }
}

test("what a run costs is recorded against the person", async () => {
  const { deps, turns } = setup([{ say: "sunny" }]);
  const ledger = new Ledger();

  await answer(ASKED, { ...deps, usage: ledger });

  assert.equal(ledger.recorded.length, 1);
  assert.equal(ledger.recorded[0]!.userId, "usr_priya");
  assert.equal(ledger.recorded[0]!.agentId, AGENT.id);
  assert.equal(turns.rows.length, 2);
});

test("past the day's ceiling nothing is spent, and the person is told why", async () => {
  const { hub, turns, harness, deps } = setup([{ say: "sunny" }]);
  const ledger = new Ledger();
  ledger.allow = false;

  await answer(ASKED, { ...deps, usage: ledger });

  // The model was never called, so nothing was spent — which is the point of
  // checking before the run rather than during it.
  assert.equal(harness.runs.length, 0);
  assert.equal(ledger.recorded.length, 0);
  // Nothing was written down either: there was no turn.
  assert.equal(turns.rows.length, 0);
  // And the person is not left staring at silence.
  assert.match(hub.said() + hub.calls.join(" "), /reached today|send/i);
});

/**
 * The claim the whole shape of this service rests on: one process answers for
 * every agent, of every person, and never confuses two of them.
 *
 * Until the translator there was one agent, so none of this had run twice.
 */
test("two agents in one process keep their own persona, tools and memory", async () => {
  const weatherTool: Tool = {
    name: "weather",
    description: "",
    parameters: {},
    execute: async () => ({ highC: 31 }),
  };
  const hub = new StubHub();
  const turns = new MemoryTurns();
  const harness = new FakeHarness([{ say: "answered" }]);
  const deps = {
    turns,
    harness,
    tools: new Map([[weatherTool.name, () => weatherTool]]),
    client: hub.asClient(),
  };

  const translator: AgentRecord = {
    ...AGENT,
    id: "00000000-0000-0000-0000-000000000002",
    hubAgentId: "agt_2",
    persona: "You translate.",
    template: { id: "translator", name: "Translator", persona: "You translate.", tools: [], model: "", starters: [] },
  };

  await answer({ ...ASKED, agent: AGENT, conversationId: "cnv_w" }, deps);
  await answer({ ...ASKED, agent: translator, conversationId: "cnv_t" }, deps);

  // Each was run with its own words...
  assert.equal(harness.runs[0]!.system, "You are the weather.");
  assert.equal(harness.runs[1]!.system, "You translate.");
  // ...and its own tools: the translator has none, and must not be handed the
  // weather one merely because the process knows about it.
  assert.deepEqual(harness.runs[0]!.tools.map((t) => t.name), ["weather"]);
  assert.deepEqual(harness.runs[1]!.tools, []);

  // What was written down belongs to one agent and one conversation each.
  const weatherRows = turns.rows.filter((r) => r.agentId === AGENT.id);
  const translatorRows = turns.rows.filter((r) => r.agentId === translator.id);
  assert.equal(weatherRows.length, 2);
  assert.equal(translatorRows.length, 2);
  assert.ok(weatherRows.every((r) => r.conversationId === "cnv_w"));
  assert.ok(translatorRows.every((r) => r.conversationId === "cnv_t"));
});

test("the model is told the person's clock, so 'tomorrow at 7' is theirs", () => {
  const at = new Date("2026-09-11T06:30:00Z"); // 07:30 in London, 12:00 in Kolkata
  const london = withClock("You are the weather.", "Europe/London", at);
  assert.match(london, /time zone is Europe\/London/);
  assert.match(london, /07:30/);
  assert.match(withClock("You are the weather.", "Asia/Kolkata", at), /12:00/);
  // No phone has said: the persona is left as it is.
  assert.equal(withClock("You are the weather.", undefined, at), "You are the weather.");
});
