/**
 * Every tool this runtime knows, by name, as something built per run.
 *
 * A template names the tools its agents may use; anything not here is simply
 * not offered. They are factories rather than values because a tool that
 * touches a person's own things — their routines, later their mail — is given
 * the conversation it was called in, and so cannot reach outside it however
 * the model asks.
 */

import type { Actions } from "../actions.ts";
import type { ToolFactory } from "../harness/harness.ts";
import type { Routines } from "../routines.ts";
import { createRoutineTool, deleteRoutineTool, listRoutinesTool } from "./routines.ts";
import { weatherTool } from "./weather.ts";

export interface ToolDeps {
  routines: Routines;
  actions: Actions;
}

export function builtinTools(deps: ToolDeps): Map<string, ToolFactory> {
  const tools: Record<string, ToolFactory> = {
    // Weather needs nothing about who is asking.
    weather: () => weatherTool(),
    create_routine: (context) => createRoutineTool(deps, context),
    list_routines: (context) => listRoutinesTool(deps, context),
    delete_routine: (context) => deleteRoutineTool(deps, context),
  };
  return new Map(Object.entries(tools));
}
