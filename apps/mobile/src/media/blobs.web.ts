// Copies of files this browser has fetched, so a picture already looked at
// is there with no signal. One IndexedDB store of blobs by media id.

const DB = 'cuckoo-media';
const STORE = 'blobs';

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

export async function getBlob(key: string): Promise<Blob | null> {
  try {
    const value = await run<unknown>('readonly', (s) => s.get(key));
    return value instanceof Blob ? value : null;
  } catch {
    return null;
  }
}

export async function putBlob(key: string, blob: Blob): Promise<void> {
  try {
    await run('readwrite', (s) => s.put(blob, key));
  } catch {
    // A browser that will not keep it shows it anyway.
  }
}

export async function clearBlobs(): Promise<void> {
  try {
    await run('readwrite', (s) => s.clear());
  } catch {
    // Nothing kept, nothing to clear.
  }
}
