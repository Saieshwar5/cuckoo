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

// initials is the avatar text for a name: up to two letters.
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + second).toUpperCase();
}
