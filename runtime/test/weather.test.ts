/**
 * The weather tool, against a stubbed Open-Meteo.
 *
 * What is checked is the shape handed to the model: four parallel arrays are
 * how the service answers, and one object per day is what a model reads
 * without mistakes.
 */

import assert from "node:assert/strict";
import { test } from "node:test";

import { weatherTool } from "../src/tools/weather.ts";

function stubFetch(routes: { geocode?: unknown; forecast?: unknown; fail?: boolean }) {
  return (async (url: string | URL) => {
    const href = String(url);
    if (routes.fail) return new Response("no", { status: 500 });
    const body = href.includes("geocoding") ? routes.geocode : routes.forecast;
    return new Response(JSON.stringify(body), { status: 200 });
  }) as unknown as typeof fetch;
}

const HYDERABAD = {
  results: [
    { name: "Hyderabad", country: "India", latitude: 17.4, longitude: 78.5, timezone: "Asia/Kolkata" },
  ],
};

const FORECAST = {
  daily: {
    time: ["2026-09-09", "2026-09-10"],
    temperature_2m_max: [31.2, 29.8],
    temperature_2m_min: [23.0, 22.4],
    precipitation_probability_max: [10, 80],
    weather_code: [1, 61],
  },
};

test("a place becomes a forecast the model can read", async () => {
  const tool = weatherTool(stubFetch({ geocode: HYDERABAD, forecast: FORECAST }));
  const result = (await tool.execute({ place: "Hyderabad", days: 2 })) as Record<string, unknown>;

  assert.equal(result.place, "Hyderabad, India");
  assert.deepEqual(result.forecast, [
    { date: "2026-09-09", highC: 31.2, lowC: 23.0, rainChancePercent: 10, conditions: "mainly clear" },
    { date: "2026-09-10", highC: 29.8, lowC: 22.4, rainChancePercent: 80, conditions: "rain" },
  ]);
});

test("a place nobody has heard of is said so, not thrown", async () => {
  const tool = weatherTool(stubFetch({ geocode: { results: [] } }));
  const result = (await tool.execute({ place: "Nowhereville" })) as { error?: string };
  assert.match(result.error ?? "", /no place called Nowhereville/);
});

test("no place at all is refused before any request", async () => {
  const tool = weatherTool(stubFetch({}));
  const result = (await tool.execute({})) as { error?: string };
  assert.match(result.error ?? "", /no place/);
});

test("a service that is down becomes an answer, not a crash", async () => {
  const tool = weatherTool(stubFetch({ fail: true }));
  const result = (await tool.execute({ place: "Hyderabad" })) as { error?: string };
  assert.ok(result.error);
});

test("days are held to what the service offers", async () => {
  let asked = "";
  const fetchImpl = (async (url: string | URL) => {
    const href = String(url);
    if (href.includes("geocoding")) return new Response(JSON.stringify(HYDERABAD));
    asked = new URL(href).searchParams.get("forecast_days") ?? "";
    return new Response(JSON.stringify(FORECAST));
  }) as unknown as typeof fetch;

  const tool = weatherTool(fetchImpl);
  await tool.execute({ place: "Hyderabad", days: 99 });
  assert.equal(asked, "7");
  await tool.execute({ place: "Hyderabad", days: 0 });
  assert.equal(asked, "1");
});
