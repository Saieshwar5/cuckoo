/**
 * Setting a routine by saying so.
 *
 * "Every morning at 8, tell me the weather" is a sentence, not a form. The
 * model turns it into a repeat and a time; this turns them back into words and
 * asks the person to agree before anything is written down — because a model
 * that mishears "8am" as "8pm" should cost a tap, not a night. Once set, the
 * routine appears in the app beside the ones made there.
 */

import type { Actions } from "../actions.ts";
import type { Tool, ToolContext } from "../harness/harness.ts";
import { describe, validCadence, type Cadence } from "../cadence.ts";
import type { Routines } from "../routines.ts";

const DEFAULT_TZ = "Asia/Kolkata";

export interface RoutineToolDeps {
  routines: Routines;
  actions: Actions;
}

/** Offer to set a routine. Nothing is written until the person taps. */
export function createRoutineTool(deps: RoutineToolDeps, context: ToolContext): Tool {
  return {
    name: "create_routine",
    activity: "Getting that ready",
    description:
      "Offer to do something on a schedule: every morning, every weekday, every Friday, " +
      "or once at a set time. The person must confirm before it is set; you are offering, " +
      "not doing. Use it whenever someone asks to be reminded or told something regularly.",
    parameters: {
      type: "object",
      properties: {
        instruction: {
          type: "string",
          description:
            "What to do each time, written as the thing to do rather than as a " +
            "request to set it up: 'tell me the weather in Hyderabad', not 'remind me " +
            "to check the weather'. It is run when the routine fires.",
        },
        title: {
          type: "string",
          description: "A short name the person sees in their list, e.g. 'Morning weather'.",
        },
        repeat: { type: "string", enum: ["once", "daily", "weekdays", "weekly"] },
        time: { type: "string", description: "24-hour clock, 'HH:MM': '07:00', '18:30'." },
        days: {
          type: "array",
          items: { type: "string", enum: ["mon", "tue", "wed", "thu", "fri", "sat", "sun"] },
          description: "Weekly only: which days.",
        },
        date: { type: "string", description: "Once only: 'YYYY-MM-DD'." },
        timezone: {
          type: "string",
          description: "An IANA time zone. Leave it out to use the person's own, which is almost always right.",
        },
      },
      required: ["instruction", "title", "repeat", "time"],
    },
    async execute(args) {
      const instruction = String(args.instruction ?? "").trim();
      const title = String(args.title ?? "").trim().slice(0, 60);
      const cadence: Cadence = {
        repeat: String(args.repeat ?? "") as Cadence["repeat"],
        time: String(args.time ?? "").trim(),
        // The person's own clock unless they named another; India's only when
        // no phone has told the hub where it is.
        timezone: String(args.timezone ?? "") || context.timezone || DEFAULT_TZ,
        ...(Array.isArray(args.days) ? { days: (args.days as unknown[]).map((d) => String(d).toLowerCase()) } : {}),
        ...(args.date ? { date: String(args.date) } : {}),
      };

      if (!instruction) return { error: "the routine needs an instruction" };
      // Checked before a person is asked to agree to it: a time that cannot
      // be read is a question nobody should be shown.
      if (!validCadence(cadence)) return { error: "that is not a time I can set" };
      if (cadence.repeat === "once" && isPast(cadence)) return { error: "that time has already passed" };

      const buttonId = await deps.actions.offer({
        agentId: context.agentId,
        conversationId: context.conversationId,
        userId: context.userId,
        action: "create_routine",
        arguments: { instruction, title, cadence },
      });

      return {
        // What the model should tell the person, in its own words and their
        // language — the words are its job, the button is not.
        proposed: { instruction, when: describe(cadence), timezone: cadence.timezone },
        confirm: { buttonId, label: "Set it", cancelLabel: "No" },
      };
    },
  };
}

/** Whether a one-off's moment has gone, on the clock where it was set. */
function isPast(cadence: Cadence): boolean {
  const now = new Intl.DateTimeFormat("en-CA", {
    timeZone: cadence.timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(new Date()); // "2026-09-11, 08:05"
  const [today, clock] = now.split(", ");
  return `${cadence.date} ${cadence.time}` <= `${today} ${clock}`;
}

/** What is already running here. Reading needs no confirmation. */
export function listRoutinesTool(deps: RoutineToolDeps, context: ToolContext): Tool {
  return {
    name: "list_routines",
    activity: "Looking at your routines",
    description: "The routines already set in this conversation.",
    parameters: { type: "object", properties: {} },
    async execute() {
      const routines = await deps.routines.listFor(context.agentId, context.conversationId);
      return {
        routines: routines.map((r) => ({
          id: r.id,
          title: r.title,
          instruction: r.instruction,
          timezone: r.timezone,
          nextRun: r.nextRun,
          paused: r.paused,
        })),
      };
    },
  };
}

/** Offer to stop one. Ending something a person set up is still a change. */
export function deleteRoutineTool(deps: RoutineToolDeps, context: ToolContext): Tool {
  return {
    name: "delete_routine",
    activity: "Finding that routine",
    description: "Offer to stop a routine. Use list_routines first to find its id.",
    parameters: {
      type: "object",
      properties: { id: { type: "string", description: "The routine's id." } },
      required: ["id"],
    },
    async execute(args) {
      const id = String(args.id ?? "");
      const routines = await deps.routines.listFor(context.agentId, context.conversationId);
      const found = routines.find((r) => r.id === id);
      if (!found) return { error: "there is no routine with that id here" };

      const buttonId = await deps.actions.offer({
        agentId: context.agentId,
        conversationId: context.conversationId,
        userId: context.userId,
        action: "delete_routine",
        arguments: { id },
      });
      return {
        proposed: { stopping: found.instruction },
        confirm: { buttonId, label: "Stop it", cancelLabel: "Keep it" },
      };
    },
  };
}
