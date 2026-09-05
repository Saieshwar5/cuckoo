// A handle is an agent's permanent address on a hub: lowercase, letters and
// digits and dashes and underscores, three to thirty-two long, starting with
// a letter or a digit. The same rule the hub enforces.
export const handlePattern = /^[a-z0-9][a-z0-9_-]{2,31}$/;

export function isValidHandle(handle: string): boolean {
  return handlePattern.test(handle);
}

// suggestHandle turns a display name into a handle to start from: accents
// stripped, spaces to dashes, and padded when the name is too short to be
// one on its own.
export function suggestHandle(name: string): string {
  const base = name
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^[-_]+/, '')
    .replace(/[-_]+$/, '')
    .replace(/-{2,}/g, '-')
    .slice(0, 32)
    .replace(/[-_]+$/, '');
  if (base.length === 0) return '';
  return base.length >= 3 ? base : `${base}-agent`.slice(0, 32);
}
