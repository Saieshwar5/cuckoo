// A pair code is the secret in a link: pair_ followed by the token. It
// arrives as a scanned URL, a pasted URL, a link on the app's own scheme,
// or the bare code; all of them mean the same thing.
const codePattern = /^pair_[a-z0-9]{8,}$/;

export function parsePairCode(input: string): string | null {
  const s = input.trim();
  if (!s) return null;
  if (codePattern.test(s)) return s;
  const m = /\/p\/(pair_[a-z0-9]+)(?:[/?#]|$)/.exec(s);
  if (m?.[1]) return m[1];
  return null;
}
