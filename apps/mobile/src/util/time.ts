// formatListTime renders a timestamp the way a chat list does: the time
// today, "Yesterday" yesterday, and a short date before that.
export function formatListTime(iso: string, now: Date = new Date(), yesterdayLabel = 'Yesterday'): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const sameDay = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  if (sameDay(d, now)) {
    return d.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', hour12: false });
  }
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(d, yesterday)) return yesterdayLabel;
  return d.toLocaleDateString('en-IN', { day: '2-digit', month: '2-digit', year: '2-digit' });
}

// formatCountdown renders seconds as m:ss, the way a resend timer reads.
export function formatCountdown(seconds: number): string {
  const s = Math.max(0, Math.ceil(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

// sameDay says whether two timestamps fall on the same local day.
export function sameDay(a: string, b: string): boolean {
  const x = new Date(a);
  const y = new Date(b);
  return x.getFullYear() === y.getFullYear() && x.getMonth() === y.getMonth() && x.getDate() === y.getDate();
}

// formatClock is the time inside a bubble: 24-hour, as phones here show it.
export function formatClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit', hour12: false });
}

// formatDay is the divider between days in a conversation.
export function formatDay(
  iso: string,
  now: Date = new Date(),
  labels: { today: string; yesterday: string } = { today: 'Today', yesterday: 'Yesterday' },
): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  if (sameDay(iso, now.toISOString())) return labels.today;
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(iso, yesterday.toISOString())) return labels.yesterday;
  return d.toLocaleDateString('en-IN', { day: 'numeric', month: 'long', year: 'numeric' });
}

// Since is how long ago a moment was, in the largest unit that still says
// something: minutes within the hour, hours within the day, "yesterday",
// days within the week, and a date beyond that.
export type Since =
  | { unit: 'now' }
  | { unit: 'minutes'; count: number }
  | { unit: 'hours'; count: number }
  | { unit: 'yesterday' }
  | { unit: 'days'; count: number }
  | { unit: 'date'; date: string };

// timeSince measures the distance back to a timestamp and leaves the words
// to the caller, which is the only part that knows the sentence it is in.
// A moment in the future — a device whose clock runs ahead of ours — is
// "now" rather than a negative count. An unreadable timestamp is null.
export function timeSince(iso: string, now: Date = new Date()): Since | null {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  const minutes = Math.floor((now.getTime() - d.getTime()) / 60_000);
  if (minutes < 1) return { unit: 'now' };
  if (minutes < 60) return { unit: 'minutes', count: minutes };
  // Whole days apart, not hours divided by 24: eleven last night and one
  // this afternoon is yesterday, however few hours lie between them.
  const startOfDay = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.round((startOfDay(now) - startOfDay(d)) / 86_400_000);
  if (days === 0) return { unit: 'hours', count: Math.floor(minutes / 60) };
  if (days === 1) return { unit: 'yesterday' };
  if (days < 7) return { unit: 'days', count: days };
  return {
    unit: 'date',
    date: d.toLocaleDateString('en-IN', {
      day: 'numeric',
      month: 'long',
      ...(d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' }),
    }),
  };
}

// formatDuration is how long a recording or a video runs, as it is
// labelled on one: minutes and seconds, counting from zero.
export function formatDuration(seconds: number): string {
  const whole = Math.max(0, Math.round(seconds));
  return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
}
