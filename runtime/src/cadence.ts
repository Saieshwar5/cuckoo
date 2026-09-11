/**
 * When, as the hub and the app write it, and as this runtime's timer reads it.
 *
 * The app sends structure — a repeat, a time, a zone — so nothing is guessed.
 * The timer here speaks cron. This is the translation, both ways, and the
 * words the person is shown before they agree to anything.
 */

export interface Cadence {
  repeat: "once" | "daily" | "weekdays" | "weekly";
  /** "07:00", 24-hour, in `timezone`. */
  time: string;
  /** Weekly only: "mon" … "sun". */
  days?: string[];
  /** Once only: "2026-09-12". */
  date?: string;
  timezone: string;
}

const DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];

/** The cron line for a cadence. A one-off becomes a date, run once. */
export function cronOf(cadence: Cadence): string {
  const [hour, minute] = cadence.time.split(":").map((part) => Number(part));
  const at = `${minute ?? 0} ${hour ?? 0}`;
  switch (cadence.repeat) {
    case "daily":
      return `${at} * * *`;
    case "weekdays":
      return `${at} * * 1-5`;
    case "weekly":
      return `${at} * * ${(cadence.days ?? []).map((d) => DAYS.indexOf(d)).filter((d) => d >= 0).join(",")}`;
    case "once": {
      const [, month, day] = (cadence.date ?? "").split("-").map((part) => Number(part));
      return `${at} ${day} ${month} *`;
    }
  }
}

/**
 * The cadence a cron line means, when it is one the app can show: routines
 * set before the app could see them are carried across this way. Anything
 * cleverer than a time on some days of the week is left where it is.
 */
export function cadenceOf(cron: string, timezone: string): Cadence | undefined {
  const match = /^(\d{1,2}) (\d{1,2}) \* \* (\*|1-5|[0-6](?:,[0-6])*)$/.exec(cron.trim());
  if (!match) return undefined;
  const [, minute, hour, days] = match;
  const time = `${hour!.padStart(2, "0")}:${minute!.padStart(2, "0")}`;
  if (Number(hour) > 23 || Number(minute) > 59) return undefined;
  if (days === "*") return { repeat: "daily", time, timezone };
  if (days === "1-5") return { repeat: "weekdays", time, timezone };
  return { repeat: "weekly", time, days: [...new Set(days!.split(",").map((d) => DAYS[Number(d)]!))], timezone };
}

/** The cadence in plain words, for saying back before anything is set. */
export function describe(cadence: Cadence): string {
  const [hour, minute] = cadence.time.split(":").map((part) => Number(part));
  const h = hour ?? 0;
  const clock = `${h % 12 || 12}:${String(minute ?? 0).padStart(2, "0")} ${h < 12 ? "am" : "pm"}`;
  switch (cadence.repeat) {
    case "daily":
      return `every day at ${clock}`;
    case "weekdays":
      return `every weekday at ${clock}`;
    case "weekly":
      return `every ${(cadence.days ?? []).join(", ")} at ${clock}`;
    case "once":
      return `on ${cadence.date} at ${clock}`;
  }
}

/** Whether a cadence is one this runtime can hold. */
export function validCadence(cadence: Cadence): boolean {
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(cadence.time)) return false;
  if (cadence.repeat === "weekly") return (cadence.days ?? []).every((d) => DAYS.includes(d)) && !!cadence.days?.length;
  if (cadence.repeat === "once") return /^\d{4}-\d{2}-\d{2}$/.test(cadence.date ?? "");
  return cadence.repeat === "daily" || cadence.repeat === "weekdays";
}
