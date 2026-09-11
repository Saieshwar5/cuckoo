/**
 * Keeping routines and the hub's schedules in step.
 *
 * The routine is what runs; the schedule is what the person sees in the app,
 * and what they pause and delete. Neither side is allowed to drift: a routine
 * made in the chat is told to the hub, and whatever the person does in the
 * app is done to the routine. The hub fires nothing, so if this file is
 * wrong, a schedule the person can see simply never runs.
 */

import { HubClient, parseSchedule, type Schedule, type ScheduleChange } from "@cuckoo/agent";

import type { Registry } from "./agents/registry.ts";
import { cadenceOf, cronOf, validCadence, type Cadence } from "./cadence.ts";
import type { Job } from "./jobs.ts";
import type { Routine, Routines } from "./routines.ts";

/** A name for the app when nobody gave one: the first words of the instruction. */
export function titleOf(instruction: string): string {
  const line = instruction.split("\n")[0]!.trim();
  return line.length <= 60 ? line : `${line.slice(0, 59)}…`;
}

/**
 * Tell the hub about a routine this runtime holds, so the person sees it.
 *
 * An agent the hub has not been told takes schedules keeps its routines all
 * the same; they are just not in the app. That is logged, not fatal.
 */
export async function mirror(routine: Routine, cadence: Cadence, client: HubClient, routines: Routines): Promise<boolean> {
  try {
    const schedule = await client.createSchedule(routine.conversationId, {
      title: routine.title || titleOf(routine.instruction),
      instruction: routine.instruction,
      cadence,
    });
    await routines.setHubId(routine.id, schedule.id);
    return true;
  } catch (error) {
    console.warn(`runtime: routine ${routine.id} is not shown in the app`, error);
    return false;
  }
}

/**
 * Carry routines made before the app could see them across to the hub, once,
 * at start. Anything whose cron line is cleverer than the app can show stays
 * where it is.
 */
export async function mirrorExisting(registry: Registry, routines: Routines, hubUrl: string): Promise<number> {
  let mirrored = 0;
  for (const routine of await routines.unmirrored()) {
    const cadence = cadenceOf(routine.schedule, routine.timezone);
    if (!cadence) continue;
    const agent = await registry.findById(routine.agentId);
    const secret = agent ? await registry.secretFor(agent.hubAgentId) : undefined;
    if (!secret) continue;
    if (await mirror(routine, cadence, new HubClient(secret, { hub: hubUrl }), routines)) mirrored++;
  }
  return mirrored;
}

interface ScheduleJobDeps {
  registry: Registry;
  routines: Routines;
  hubUrl: string;
}

/** A schedule event from the hub: what the person did to a schedule in the app. */
export async function handleScheduleJob(job: Job, deps: ScheduleJobDeps): Promise<void> {
  const agent = await deps.registry.findById(job.agentId);
  if (!agent) return;
  const secret = await deps.registry.secretFor(agent.hubAgentId);
  if (!secret) return;
  const client = new HubClient(secret, { hub: deps.hubUrl });

  const data = job.payload as {
    conversation?: { id?: string };
    participants?: { kind?: string; id?: string }[];
    schedule?: unknown;
  };
  const conversationId = data.conversation?.id ?? "";
  const userId = data.participants?.find((p) => p.kind === "user")?.id ?? "";
  const change: ScheduleChange = {
    type: job.type.slice("schedule.".length) as ScheduleChange["type"],
    schedule: parseSchedule(data.schedule),
  };
  await applyChange(change, { agentId: agent.id, conversationId, userId }, client, deps.routines);
}

/** What the person did in the app, done to the routine that runs it. */
export async function applyChange(
  change: ScheduleChange,
  where: { agentId: string; conversationId: string; userId: string },
  client: HubClient,
  routines: Routines,
): Promise<void> {
  const schedule = change.schedule;
  const existing = await routines.findByHubId(schedule.id);

  if (change.type === "deleted") {
    if (existing) await routines.delete(existing.id, where.agentId);
    return;
  }
  if (!validCadence(schedule.cadence)) {
    await client.deleteSchedule(where.conversationId, schedule.id).catch(() => {});
    await client
      .send(where.conversationId, "I can't keep that schedule — its time is not one I can read. Try another?")
      .catch(() => {});
    return;
  }

  // The what and when come first, whatever the status: a paused schedule
  // still moves when its person's clock does, and resumes on the new one.
  const shape = routineShape(schedule);
  if (existing) {
    await routines.change(existing.id, shape);
  } else if (schedule.status !== "paused") {
    await routines.create({
      agentId: where.agentId,
      conversationId: where.conversationId,
      userId: where.userId,
      ...shape,
      hubScheduleId: schedule.id,
    });
  }

  if (schedule.status === "paused") {
    if (existing && !existing.paused) await routines.pause(existing.id, "paused in the app");
    return;
  }
  if (existing?.paused) await routines.resume(existing.id);
  // Pending: new, or a new what or when. Held now, and said so.
  if (schedule.status === "pending") {
    await client.updateSchedule(where.conversationId, schedule.id, { status: "active" });
  }
}

function routineShape(schedule: Schedule) {
  return {
    instruction: schedule.instruction,
    schedule: cronOf(schedule.cadence),
    timezone: schedule.cadence.timezone,
    once: schedule.cadence.repeat === "once",
    title: schedule.title,
  };
}
