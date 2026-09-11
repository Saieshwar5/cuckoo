/**
 * Setting a routine by saying so.
 *
 * "Every morning at 8, tell me the weather" is a sentence, not a form. The
 * model turns it into a schedule; this turns the schedule back into words and
 * asks the person to agree before anything is written down — because a model
 * that mishears "8am" as "every 8 minutes" should cost a tap, not a night.
 */

import type { Actions } from "../actions.ts";
import type { Tool, ToolContext } from "../harness/harness.ts";
import { nextRun, validSchedule, type Routines } from "../routines.ts";

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
      "Offer to run an instruction on a schedule, for example every morning or " +
      "every Friday. The person must confirm before it is set; you are offering, " +
      "not doing. Use it when someone asks to be reminded or told something regularly.",
    parameters: {
      type: "object",
      properties: {
        instruction: {
          type: "string",
          description:
            "What to do each time, written as the thing to do rather than as a " +
            "request to set it up: 'tell me to take my tablets', not 'remind me " +
            "to take my tablets'. It is run when the routine fires.",
        },
        schedule: {
          type: "string",
          description:
            "A five-field cron expression, e.g. '0 8 * * *' for 08:00 every day, " +
            "'30 18 * * 5' for 18:30 on Fridays.",
        },
        timezone: {
          type: "string",
          description: `An IANA time zone. Defaults to ${DEFAULT_TZ}.`,
        },
      },
      required: ["instruction", "schedule"],
    },
    async execute(args) {
      const instruction = String(args.instruction ?? "").trim();
      const schedule = String(args.schedule ?? "").trim();
      const timezone = String(args.timezone ?? "") || DEFAULT_TZ;

      if (!instruction) return { error: "the routine needs an instruction" };
      // Checked before a person is asked to agree to it: a schedule that
      // cannot be read is a question nobody should be shown.
      if (!validSchedule(schedule, timezone)) {
        return { error: `'${schedule}' is not a schedule I can read, in ${timezone}` };
      }

      const first = nextRun(schedule, timezone);
      const buttonId = await deps.actions.offer({
        agentId: context.agentId,
        conversationId: context.conversationId,
        userId: context.userId,
        action: "create_routine",
        arguments: { instruction, schedule, timezone },
      });

      return {
        // What the model should tell the person, in its own words and their
        // language — the words are its job, the button is not.
        proposed: { instruction, firstRun: first.toISOString(), timezone },
        confirm: { buttonId, label: "Set it", cancelLabel: "No" },
      };
    },
  };
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
          instruction: r.instruction,
          schedule: r.schedule,
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
