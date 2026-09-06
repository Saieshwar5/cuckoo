import * as SecureStore from 'expo-secure-store';

// Which look the person asked for, or none: follow the phone. Kept in the
// same place as the session, not because it is secret but because it is
// the one store this app already has that needs no user id to open.
export type Preference = 'system' | 'light' | 'dark';

const KEY = 'cuckoo.theme';

export async function loadPreference(): Promise<Preference> {
  try {
    const raw = await SecureStore.getItemAsync(KEY);
    return raw === 'light' || raw === 'dark' ? raw : 'system';
  } catch {
    return 'system';
  }
}

export async function savePreference(p: Preference): Promise<void> {
  try {
    if (p === 'system') await SecureStore.deleteItemAsync(KEY);
    else await SecureStore.setItemAsync(KEY, p);
  } catch {
    // Not remembered; the choice still holds until the app is closed.
  }
}
