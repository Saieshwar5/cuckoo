import type { Frame, Schedule } from '../api/types';

// A conversation's schedules, and how live frames change them. Pure, so it
// is tested without a screen.

function byCreated(a: Schedule, b: Schedule): number {
  return a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0;
}

// upsertSchedule puts a schedule in the list, or replaces the one it is.
export function upsertSchedule(list: Schedule[], s: Schedule): Schedule[] {
  const i = list.findIndex((x) => x.id === s.id);
  if (i < 0) return [...list, s].sort(byCreated);
  const next = list.slice();
  next[i] = s;
  return next;
}

export function removeSchedule(list: Schedule[], id: string): Schedule[] {
  return list.some((x) => x.id === id) ? list.filter((x) => x.id !== id) : list;
}

// applyScheduleFrame folds one live frame into a conversation's schedules.
export function applyScheduleFrame(list: Schedule[], frame: Frame, conversationId: string): Schedule[] {
  if (frame.type !== 'schedule.changed' || frame.data.conversation_id !== conversationId) return list;
  return frame.data.deleted
    ? removeSchedule(list, frame.data.schedule.id)
    : upsertSchedule(list, frame.data.schedule);
}
