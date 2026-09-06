// What the app remembers between opens.
//
// The hub is the record. This is a copy of the parts of it this device has
// seen, kept so the list and every chat opened before are on screen the
// moment the app is, and still readable when there is no signal. Nothing
// here is ever the truth about a conversation; it is what the truth looked
// like the last time it was fetched, corrected the moment it is fetched
// again.
//
// It is a key-value store of JSON documents, and deliberately no more.
// One document per thing a screen loads — the chat list, one chat's
// messages, the queue of unsent sends — read whole and written whole. A
// table per kind of row would let older history be paged offline; nobody
// has asked for that, and a document per screen is what makes this a few
// dozen lines on each platform rather than a schema.

export interface Cache {
  get<T>(key: string): Promise<T | null>;
  set(key: string, value: unknown): Promise<void>;
  remove(key: string): Promise<void>;
  // Forgets everything. Signing out calls this: a shared phone should not
  // carry someone else's conversations.
  clear(): Promise<void>;
}

// Keys. Every one is under the person's id, so two accounts on one device
// never read each other's documents even before a sign-out wiped them.
export const keys = {
  conversations: (userId: string) => `u:${userId}:conversations`,
  messages: (userId: string, conversationId: string) => `u:${userId}:messages:${conversationId}`,
  agents: (userId: string) => `u:${userId}:agents`,
  outbox: (userId: string) => `u:${userId}:outbox`,
  // Half-written words in one chat, kept until sent or erased.
  draft: (userId: string, conversationId: string) => `u:${userId}:draft:${conversationId}`,
};

// How many messages of one chat are kept. Everything ever loaded, up to a
// bound, so a busy chat with an agent does not grow without limit.
export const MESSAGES_KEPT = 500;

// MemoryCache forgets when the process does. It is what tests use, and what
// a platform with no storage falls back to: the app then behaves exactly as
// it did before any of this existed.
export class MemoryCache implements Cache {
  private items = new Map<string, string>();

  async get<T>(key: string): Promise<T | null> {
    const raw = this.items.get(key);
    return raw === undefined ? null : (JSON.parse(raw) as T);
  }

  async set(key: string, value: unknown): Promise<void> {
    this.items.set(key, JSON.stringify(value));
  }

  async remove(key: string): Promise<void> {
    this.items.delete(key);
  }

  async clear(): Promise<void> {
    this.items.clear();
  }
}

// guarded wraps a cache so a storage failure is never a screen failure. A
// phone that is out of space, or a browser in a private window, gets an
// app that works and does not remember, which is the app as it was.
export function guarded(inner: Cache, onError: (err: unknown) => void = () => {}): Cache {
  const attempt = async <T>(fn: () => Promise<T>, fallback: T): Promise<T> => {
    try {
      return await fn();
    } catch (err) {
      onError(err);
      return fallback;
    }
  };
  return {
    get: (key) => attempt(() => inner.get(key), null),
    set: (key, value) => attempt(() => inner.set(key, value), undefined),
    remove: (key) => attempt(() => inner.remove(key), undefined),
    clear: () => attempt(() => inner.clear(), undefined),
  };
}
