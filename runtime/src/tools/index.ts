/**
 * Every tool this runtime knows, by name.
 *
 * A template names the tools its agents may use; anything not in this map is
 * simply not offered. Adding a tool is a line here and a row in `templates` —
 * never a migration, and never a change to the loop that runs them.
 */

import type { Tool } from "../harness/harness.ts";
import { weatherTool } from "./weather.ts";

export function builtinTools(): Map<string, Tool> {
  const tools = [weatherTool()];
  return new Map(tools.map((tool) => [tool.name, tool]));
}
