/**
 * The worker that lets an agent speak first.
 *
 * A routine is the same conversation on a timer, so a due routine runs the
 * ordinary answer loop with the person's own instruction as the input. What is
 * different is that nobody is waiting: there is no message to reply to, the
 * person may be asleep, and until push notifications exist they will not know
 * until they open the app.
 *
 * Two things are therefore checked before anything is sent, and both live here
 * rather than in a prompt: the person's daily budget for being spoken to, and
 * whether the agent still exists.
 */

import { HubClient, type InFlight } from "@cuckoo/agent";

import type { Actions } from "./actions.ts";
import type { Registry } from "./agents/registry.ts";
import { answer } from "./answer.ts";
import type { Harness, ToolFactory } from "./harness/harness.ts";
import type { TurnStore } from "./memory/turns.ts";
import type { Proactive } from "./proactive.ts";
import type { Routines } from "./routines.ts";
import type { Usage } from "./usage.ts";

export interface SchedulerDeps {
  routines: Routines;
  registry: Registry;
  turns: TurnStore;
  harness: Harness;
  tools: Map<string, ToolFactory>;
  usage: Usage;
  actions: Actions;
  proactive: Proactive;
  /** Runs in progress, so a person can stop a routine mid-reply too. */
  inflight?: InFlight;
  hubUrl: string;
  /** How often to look for due routines. A minute is the resolution of cron. */
  intervalMs?: number;
}

export class Scheduler {
  private readonly deps: SchedulerDeps;
  private readonly intervalMs: number;
  private running = false;
  private timer?: ReturnType<typeof setTimeout>;

  constructor(deps: SchedulerDeps) {
    this.deps = deps;
    this.intervalMs = deps.intervalMs ?? 30_000;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    void this.loop();
  }

  async stop(): Promise<void> {
    this.running = false;
    if (this.timer) clearTimeout(this.timer);
  }

  /** One pass. Returns how many routines ran. */
  async tick(): Promise<number> {
    const due = await this.deps.routines.claimDue();
    for (const routine of due) {
      try {
        await this.run(routine);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        const { paused } = await this.deps.routines.recordFailure(routine.id, message);
        console.error(`runtime: routine ${routine.id} failed${paused ? " and was paused" : ""}`, error);
        if (paused) await this.sayItStopped(routine, message);
      }
    }
    return due.length;
  }

  private async run(routine: {
    id: string;
    agentId: string;
    conversationId: string;
    userId: string;
    instruction: string;
  }): Promise<void> {
    const agent = await this.deps.registry.findById(routine.agentId);
    if (!agent) return; // deleted since the routine was set

    // Asked before the model is called, so a person over their budget costs
    // nothing rather than being answered into a message that is not sent.
    if (!(await this.deps.proactive.maySend(routine.userId))) {
      console.warn(`runtime: routine ${routine.id} held back; ${routine.userId} is at the day's limit`);
      return;
    }

    const secret = await this.deps.registry.secretFor(agent.hubAgentId);
    if (!secret) return;

    const running = this.deps.inflight?.start(routine.conversationId);
    try {
      await answer(
        {
          agent,
          conversationId: routine.conversationId,
          // The run says plainly that it is one.
          //
          // The first version of this replayed the instruction as though the
          // person had just said it, on the reasoning that nothing about the
          // answer should be different. That was wrong, and a real run showed
          // why: "remind me to take my tablets" means *set this up* the first
          // time and *tell me now* when it fires. Handed the same words with no
          // frame, the agent offered to set the routine it was already running.
          text:
            `[Your routine is running now. The person did not just say this — ` +
            `do the thing itself, briefly, and do not offer to set anything up.] ` +
            routine.instruction,
          hubMessageId: "",
          userId: routine.userId,
        },
        {
          turns: this.deps.turns,
          harness: this.deps.harness,
          tools: this.deps.tools,
          usage: this.deps.usage,
          client: new HubClient(secret, { hub: this.deps.hubUrl }),
          signal: running?.signal,
        },
      );
    } finally {
      if (running) this.deps.inflight?.end(routine.conversationId, running);
    }

    await this.deps.proactive.record({
      userId: routine.userId,
      agentId: routine.agentId,
      conversationId: routine.conversationId,
      reason: `routine:${routine.id}`,
    });
  }

  /**
   * Tell the person a routine has stopped.
   *
   * A stopped routine and a silent one look the same from the outside, and the
   * silent one is the version where they think the agent is broken.
   */
  private async sayItStopped(
    routine: { agentId: string; conversationId: string; instruction: string },
    reason: string,
  ): Promise<void> {
    try {
      const agent = await this.deps.registry.findById(routine.agentId);
      if (!agent) return;
      const secret = await this.deps.registry.secretFor(agent.hubAgentId);
      if (!secret) return;
      await new HubClient(secret, { hub: this.deps.hubUrl }).send(
        routine.conversationId,
        `I have stopped "${routine.instruction}" — it failed several times (${reason}). Ask me to set it again when you want it back.`,
      );
    } catch {
      // Saying so is a courtesy; failing to say so must not fail the pass.
    }
  }

  private async loop(): Promise<void> {
    while (this.running) {
      try {
        await this.tick();
      } catch (error) {
        console.error("runtime: scheduler pass failed", error);
      }
      await new Promise((resolve) => {
        this.timer = setTimeout(resolve, this.intervalMs);
      });
    }
  }
}
