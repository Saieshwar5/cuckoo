/**
 * Weather, from Open-Meteo: no key, no account, and a geocoder in the same
 * place — which is why it is the first tool. The point of the first agent is
 * to prove the loop, not to negotiate an API contract.
 */

import type { Tool } from "../harness/harness.ts";

const GEOCODE = "https://geocoding-api.open-meteo.com/v1/search";
const FORECAST = "https://api.open-meteo.com/v1/forecast";

interface Place {
  name: string;
  country: string;
  latitude: number;
  longitude: number;
  timezone: string;
}

export function weatherTool(fetchImpl: typeof fetch = fetch): Tool {
  return {
    name: "weather",
    description:
      "The weather forecast for a place. Use it whenever someone asks about " +
      "weather, rain, heat or what to wear. Understands Indian place names.",
    parameters: {
      type: "object",
      properties: {
        place: { type: "string", description: "A city or town, e.g. 'Hyderabad'." },
        days: {
          type: "integer",
          description: "How many days from today, 1 to 7. 1 means today only.",
          minimum: 1,
          maximum: 7,
        },
      },
      required: ["place"],
    },
    async execute(args) {
      const place = String(args.place ?? "").trim();
      if (!place) return { error: "no place was named" };
      const days = clamp(Number(args.days ?? 3), 1, 7);

      const found = await geocode(fetchImpl, place);
      if (!found) return { error: `no place called ${place} was found` };

      const query = new URLSearchParams({
        latitude: String(found.latitude),
        longitude: String(found.longitude),
        timezone: found.timezone || "auto",
        forecast_days: String(days),
        daily: "temperature_2m_max,temperature_2m_min,precipitation_probability_max,weather_code",
      });
      const response = await fetchImpl(`${FORECAST}?${query}`);
      if (!response.ok) return { error: `the forecast service answered ${response.status}` };
      const data = (await response.json()) as { daily?: Record<string, unknown[]> };
      const daily = data.daily ?? {};

      const dates = (daily.time ?? []) as string[];
      return {
        place: `${found.name}, ${found.country}`,
        // Flattened per day: a model reads this far better than four parallel
        // arrays, and it costs nothing to do here.
        forecast: dates.map((date, i) => ({
          date,
          highC: numberAt(daily.temperature_2m_max, i),
          lowC: numberAt(daily.temperature_2m_min, i),
          rainChancePercent: numberAt(daily.precipitation_probability_max, i),
          conditions: describe(numberAt(daily.weather_code, i)),
        })),
      };
    },
  };
}

async function geocode(fetchImpl: typeof fetch, place: string): Promise<Place | undefined> {
  const query = new URLSearchParams({ name: place, count: "1", language: "en", format: "json" });
  const response = await fetchImpl(`${GEOCODE}?${query}`);
  if (!response.ok) return undefined;
  const data = (await response.json()) as { results?: Record<string, unknown>[] };
  const first = data.results?.[0];
  if (!first) return undefined;
  return {
    name: String(first.name ?? place),
    country: String(first.country ?? ""),
    latitude: Number(first.latitude),
    longitude: Number(first.longitude),
    timezone: String(first.timezone ?? "auto"),
  };
}

function numberAt(list: unknown[] | undefined, index: number): number | null {
  const value = list?.[index];
  return typeof value === "number" ? value : null;
}

function clamp(n: number, low: number, high: number): number {
  if (!Number.isFinite(n)) return low;
  return Math.min(high, Math.max(low, Math.trunc(n)));
}

/**
 * WMO weather codes in words. The model could be handed the number, but then
 * every agent built on this tool would have to know the table.
 */
function describe(code: number | null): string {
  if (code === null) return "unknown";
  if (code === 0) return "clear";
  if (code === 1) return "mainly clear";
  if (code === 2) return "partly cloudy";
  if (code === 3) return "overcast";
  if (code <= 48) return "fog";
  if (code <= 57) return "drizzle";
  if (code <= 67) return "rain";
  if (code <= 77) return "snow";
  if (code <= 82) return "rain showers";
  if (code <= 86) return "snow showers";
  return "thunderstorm";
}
