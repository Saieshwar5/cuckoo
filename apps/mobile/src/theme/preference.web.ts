// The browser version: localStorage, which is what a browser has.
export type Preference = 'system' | 'light' | 'dark';

const KEY = 'cuckoo.theme';

export async function loadPreference(): Promise<Preference> {
  try {
    const raw = globalThis.localStorage?.getItem(KEY);
    return raw === 'light' || raw === 'dark' ? raw : 'system';
  } catch {
    return 'system';
  }
}

export async function savePreference(p: Preference): Promise<void> {
  try {
    if (p === 'system') globalThis.localStorage?.removeItem(KEY);
    else globalThis.localStorage?.setItem(KEY, p);
  } catch {
    // Not remembered; the choice still holds until the tab is closed.
  }
}
