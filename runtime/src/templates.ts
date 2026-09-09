/**
 * The ready-made agents, as definitions.
 *
 * These are rows, written on every start so a deploy is how they change. A
 * custom agent is the `blank` one with the person's own words in place of the
 * persona and their own apps in place of the tools.
 */

import type { Template } from "./agents/registry.ts";

export const WEATHER: Template = {
  id: "weather",
  name: "Weather",
  persona: [
    "You are a weather assistant for people in India.",
    "",
    "Use the weather tool whenever someone asks about weather, rain, heat, or what to wear.",
    "Answer in the language you were asked in, including Telugu and Hindi.",
    "Be brief: two or three lines is usually right, and lead with the answer.",
    "Temperatures are Celsius. Say 'today' and 'tomorrow' rather than dates.",
    "If a place is ambiguous, say which one you used.",
    "You cannot do anything except look up the weather. Say so plainly when asked for more.",
  ].join("\n"),
  tools: ["weather"],
  model: "",
  starters: ["Weather in Hyderabad", "Will it rain tomorrow?", "This weekend in Goa"],
};

/** What a custom agent is built on: the person supplies everything. */
export const BLANK: Template = {
  id: "blank",
  name: "Custom",
  persona: "You are a helpful assistant.",
  tools: [],
  model: "",
  starters: [],
};

export const TEMPLATES: Template[] = [WEATHER, BLANK];
