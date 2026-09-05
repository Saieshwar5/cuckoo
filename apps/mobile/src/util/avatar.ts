import { avatarColors } from '../theme/tokens';

// initials is the avatar text for a name: up to two letters.
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + second).toUpperCase();
}

// colorFor picks an avatar colour for a name. The same name always gets the
// same colour, on every screen and every device, and names spread across
// the palette rather than crowding one colour.
export function colorFor(name: string): string {
  let h = 5381;
  for (const ch of name.trim().toLowerCase()) h = (h * 33 + ch.charCodeAt(0)) >>> 0;
  return avatarColors[h % avatarColors.length] ?? avatarColors[0];
}
