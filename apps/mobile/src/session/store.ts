import * as SecureStore from 'expo-secure-store';

// What survives an app restart: the session token and who it belongs to.
// Stored in the platform keychain, never in plain preferences.
export interface StoredSession {
  token: string;
  user: { id: string; display_name: string };
}

const KEY = 'cuckoo.session';

export async function loadSession(): Promise<StoredSession | null> {
  try {
    const raw = await SecureStore.getItemAsync(KEY);
    return raw ? (JSON.parse(raw) as StoredSession) : null;
  } catch {
    return null;
  }
}

export async function saveSession(s: StoredSession): Promise<void> {
  await SecureStore.setItemAsync(KEY, JSON.stringify(s));
}

export async function clearSession(): Promise<void> {
  await SecureStore.deleteItemAsync(KEY);
}
