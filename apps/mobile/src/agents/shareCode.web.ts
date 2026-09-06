// The browser has no keychain; localStorage is what it has.
const key = (agentId: string) => `cuckoo.share.${agentId}`;

export async function loadShareCode(agentId: string): Promise<{ tokenId: string; code: string } | null> {
  try {
    const raw = window.localStorage.getItem(key(agentId));
    return raw ? (JSON.parse(raw) as { tokenId: string; code: string }) : null;
  } catch {
    return null;
  }
}

export async function saveShareCode(
  agentId: string,
  value: { tokenId: string; code: string },
): Promise<void> {
  window.localStorage.setItem(key(agentId), JSON.stringify(value));
}

export async function clearShareCode(agentId: string): Promise<void> {
  window.localStorage.removeItem(key(agentId));
}
