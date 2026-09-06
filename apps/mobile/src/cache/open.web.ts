import { type Cache } from './cache';

// The browser's cache: one IndexedDB object store of key and value.
//
// IndexedDB rather than localStorage because localStorage is a few
// megabytes and synchronous, and a chat's history is neither small nor
// something to block the page on. SQLite exists for the browser too, as a
// WebAssembly build, and behaves differently enough that a second, native
// store is the smaller risk.

const DB = 'cuckoo-cache';
const STORE = 'kv';

let opening: Promise<IDBDatabase> | null = null;

function database(): Promise<IDBDatabase> {
  if (!opening) {
    opening = new Promise((resolve, reject) => {
      const req = indexedDB.open(DB, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(STORE);
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }
  return opening;
}

function run<T>(mode: IDBTransactionMode, op: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return database().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const req = op(db.transaction(STORE, mode).objectStore(STORE));
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
      }),
  );
}

export function openCache(): Cache {
  return {
    async get<T>(key: string): Promise<T | null> {
      const raw = await run<unknown>('readonly', (s) => s.get(key));
      return typeof raw === 'string' ? (JSON.parse(raw) as T) : null;
    },
    async set(key: string, value: unknown): Promise<void> {
      // Stored as the JSON text rather than the object, so what comes back
      // is a fresh copy with no reference to anything a reducer holds.
      await run('readwrite', (s) => s.put(JSON.stringify(value), key));
    },
    async remove(key: string): Promise<void> {
      await run('readwrite', (s) => s.delete(key));
    },
    async clear(): Promise<void> {
      await run('readwrite', (s) => s.clear());
    },
  };
}
