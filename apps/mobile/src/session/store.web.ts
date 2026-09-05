// The browser version of the session store. Metro picks this file on web
// and store.ts everywhere else. A browser has no keychain; localStorage is
// what it has, and this app in a browser is a development tool.
export interface StoredSession {
  token: string;
  user: { id: string; display_name: string };
}

const KEY = 'cuckoo.session';

export async function loadSession(): Promise<StoredSession | null> {
  try {
    const raw = globalThis.localStorage?.getItem(KEY);
    return raw ? (JSON.parse(raw) as StoredSession) : null;
  } catch {
    return null;
  }
}

export async function saveSession(s: StoredSession): Promise<void> {
  globalThis.localStorage?.setItem(KEY, JSON.stringify(s));
}

export async function clearSession(): Promise<void> {
  globalThis.localStorage?.removeItem(KEY);
}
