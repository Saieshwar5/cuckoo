import type { Cadence, Schedule, Weekday } from '../api/types';
import { t } from '../i18n';

export const WEEKDAYS: Weekday[] = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

// clock writes "07:00" the way a person reads it: "7:00 AM".
export function clock(time: string): string {
  const [h = 0, m = 0] = time.split(':').map((part) => Number(part));
  return `${h % 12 || 12}:${String(m).padStart(2, '0')} ${h < 12 ? 'AM' : 'PM'}`;
}

// cadenceLabel is when a schedule runs, in a line: "Every day · 7:00 AM".
export function cadenceLabel(c: Cadence): string {
  const at = clock(c.time);
  switch (c.repeat) {
    case 'daily':
      return t('schedules.when.daily', { time: at });
    case 'weekdays':
      return t('schedules.when.weekdays', { time: at });
    case 'weekly':
      return t('schedules.when.weekly', {
        days: (c.days ?? []).map((d) => t(`schedules.day.${d}`)).join(', '),
        time: at,
      });
    case 'once':
      return t('schedules.when.once', { date: dateLabel(c.date ?? ''), time: at });
  }
}

// dateLabel writes "2026-09-12" as "12 Sep".
export function dateLabel(date: string): string {
  const [, month = 1, day = 1] = date.split('-').map((part) => Number(part));
  return `${day} ${t(`schedules.month.${month}`)}`;
}

// whenLabel is a moment as a schedule row says it: today, tomorrow, or a day.
export function whenLabel(iso: string, now: Date): string {
  const at = new Date(iso);
  const time = clock(`${at.getHours()}:${at.getMinutes()}`);
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((day(at) - day(now)) / 86_400_000);
  if (days === 0) return t('schedules.at.today', { time });
  if (days === 1) return t('schedules.at.tomorrow', { time });
  if (days === -1) return t('schedules.at.yesterday', { time });
  return t('schedules.at.day', {
    day: `${at.getDate()} ${t(`schedules.month.${at.getMonth() + 1}`)}`,
    time,
  });
}

export type Tone = 'normal' | 'quiet' | 'warn';

// statusLine is the second line of a schedule row: what is true of it now.
// A miss is said first and in red: a list that admits a miss is one people
// trust, and the hub runs nothing, so a backend that was down at seven is
// exactly what this line is for.
export function statusLine(s: Schedule, agentName: string, now: Date): { text: string; tone: Tone } {
  if (s.status === 'paused') return { text: t('schedules.status.paused'), tone: 'quiet' };
  if (s.status === 'pending')
    return { text: t('schedules.status.pending', { name: agentName }), tone: 'quiet' };
  if (s.missed_at)
    return { text: t('schedules.status.missed', { when: whenLabel(s.missed_at, now) }), tone: 'warn' };
  if (s.next_run_at)
    return { text: t('schedules.status.next', { when: whenLabel(s.next_run_at, now) }), tone: 'normal' };
  return { text: t('schedules.status.done'), tone: 'quiet' };
}

// deviceZone is where this phone's clock is, which is where "7 in the
// morning" means anything.
export function deviceZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Kolkata';
  } catch {
    return 'Asia/Kolkata';
  }
}
