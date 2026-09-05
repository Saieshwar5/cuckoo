// initials is the avatar text for a name: up to two letters.
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + second).toUpperCase();
}

// avatarIndex picks which of `count` avatar shades a name gets. The same
// name always gets the same one, on every screen and every device, and
// names spread across the shades rather than crowding one.
export function avatarIndex(name: string, count: number): number {
  let h = 5381;
  for (const ch of name.trim().toLowerCase()) h = (h * 33 + ch.charCodeAt(0)) >>> 0;
  return count > 0 ? h % count : 0;
}
