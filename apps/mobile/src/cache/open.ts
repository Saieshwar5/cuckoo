import * as SQLite from 'expo-sqlite';

import { type Cache } from './cache';

// The phone's cache: one SQLite table of key and JSON.
//
// SQLite rather than a preferences file because the documents are not
// small — a chat's last five hundred messages — and a preferences file is
// read and written whole. Rows are written one at a time and read one at
// a time, which is exactly how the documents are used.

const DB = 'cuckoo-cache.db';

let opening: Promise<SQLite.SQLiteDatabase> | null = null;

function database(): Promise<SQLite.SQLiteDatabase> {
  if (!opening) {
    opening = SQLite.openDatabaseAsync(DB).then(async (db) => {
      await db.execAsync(
        'PRAGMA journal_mode = WAL; CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY NOT NULL, value TEXT NOT NULL, updated_at INTEGER NOT NULL);',
      );
      return db;
    });
  }
  return opening;
}

export function openCache(): Cache {
  return {
    async get<T>(key: string): Promise<T | null> {
      const db = await database();
      const row = await db.getFirstAsync<{ value: string }>('SELECT value FROM kv WHERE key = ?', key);
      return row ? (JSON.parse(row.value) as T) : null;
    },
    async set(key: string, value: unknown): Promise<void> {
      const db = await database();
      await db.runAsync(
        'INSERT INTO kv (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at',
        key,
        JSON.stringify(value),
        Date.now(),
      );
    },
    async remove(key: string): Promise<void> {
      const db = await database();
      await db.runAsync('DELETE FROM kv WHERE key = ?', key);
    },
    async clear(): Promise<void> {
      const db = await database();
      await db.runAsync('DELETE FROM kv');
    },
  };
}
