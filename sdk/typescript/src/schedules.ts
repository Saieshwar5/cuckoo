/**
 * Schedules: things a person asked the agent to do at a time.
 *
 * Your backend runs them — the timer, the work, the answer. The hub shows
 * them in the app and passes on what the person does to them. Nothing fires
 * on the hub's side, so a schedule your backend is not holding does nothing.
 */

import type { HubClient } from "./client.ts";

/**
 * When a schedule runs. Structure, never a sentence: the person picked a
 * time and a repeat on their phone, so there is nothing to interpret.
 */
export interface Cadence {
  repeat: "once" | "daily" | "weekdays" | "weekly";
  /** "07:00", on a 24-hour clock, in `timezone`. */
  time: string;
  /** Weekly only: "mon" … "sun". */
  days?: string[];
  /** Once only: "2026-09-12". */
  date?: string;
  /** "Asia/Kolkata". */
  timezone: string;
}

export interface Schedule {
  id: string;
  conversationId: string;
  title: string;
  /** What to do, in the person's words. */
  instruction: string;
  cadence: Cadence;
  /** `pending` until you confirm it; `paused` only the person undoes. */
  status: "pending" | "active" | "paused";
  createdBy: "user" | "agent";
  /** Worked out by the hub from the cadence. */
  nextRunAt: string | null;
  lastRunAt: string | null;
  /** The last time it was due and nothing was sent for it. */
  missedAt: string | null;
}

/** What the person did to a schedule in the app. */
export interface ScheduleChange {
  /**
   * `requested`: hold it, then `conversation.schedules.confirm(id)` — or
   * `remove(id)` and say why. `updated`: paused, resumed, or a new time or
   * instruction, pending until you confirm again. `deleted`: stop its timer.
   */
  type: "requested" | "updated" | "deleted";
  schedule: Schedule;
}

export function parseSchedule(raw: unknown): Schedule {
  const w = (raw ?? {}) as Record<string, unknown>;
  const c = (w.cadence ?? {}) as Record<string, unknown>;
  const str = (v: unknown) => (typeof v === "string" ? v : "");
  const orNull = (v: unknown) => (typeof v === "string" ? v : null);
  return {
    id: str(w.id),
    conversationId: str(w.conversation_id),
    title: str(w.title),
    instruction: str(w.instruction),
    cadence: {
      repeat: (str(c.repeat) || "daily") as Cadence["repeat"],
      time: str(c.time),
      ...(Array.isArray(c.days) ? { days: c.days as string[] } : {}),
      ...(c.date ? { date: str(c.date) } : {}),
      timezone: str(c.timezone),
    },
    status: (str(w.status) || "pending") as Schedule["status"],
    createdBy: (str(w.created_by) || "user") as Schedule["createdBy"],
    nextRunAt: orNull(w.next_run_at),
    lastRunAt: orNull(w.last_run_at),
    missedAt: orNull(w.missed_at),
  };
}

/** The schedules in one conversation, from your side. */
export class Schedules {
  private readonly client: HubClient;
  private readonly conversationId: string;

  constructor(client: HubClient, conversationId: string) {
    this.client = client;
    this.conversationId = conversationId;
  }

  /** Your schedules in this conversation. */
  list(): Promise<Schedule[]> {
    return this.client.listSchedules(this.conversationId);
  }

  /**
   * Record a schedule you already hold — one the person asked for in the
   * chat — so they see it with the ones they made in the app.
   */
  create(input: { title: string; instruction: string; cadence: Cadence }): Promise<Schedule> {
    return this.client.createSchedule(this.conversationId, input);
  }

  /** Say you hold one the person asked for, and give it a name. */
  confirm(scheduleId: string, title?: string): Promise<Schedule> {
    return this.client.updateSchedule(this.conversationId, scheduleId, {
      status: "active",
      ...(title ? { title } : {}),
    });
  }

  update(
    scheduleId: string,
    change: { title?: string; instruction?: string; cadence?: Cadence; status?: "active" | "paused" },
  ): Promise<Schedule> {
    return this.client.updateSchedule(this.conversationId, scheduleId, change);
  }

  /** Let go of one: decline a request, or end a one-off that has run. */
  remove(scheduleId: string): Promise<void> {
    return this.client.deleteSchedule(this.conversationId, scheduleId);
  }
}
