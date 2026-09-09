/**
 * The connection pool, and the migrations that build what it talks to.
 */

import { readdir, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import pg from "pg";

export type Pool = pg.Pool;
export type PoolClient = pg.PoolClient;

export function openPool(databaseUrl: string): Pool {
  return new pg.Pool({
    connectionString: databaseUrl,
    // A runtime that cannot reach its database should say so, not hang.
    connectionTimeoutMillis: 5_000,
    max: 10,
  });
}

/**
 * Apply every migration that has not run, in filename order, each in its own
 * transaction. Plain SQL files, like the hub's — a migration you cannot read
 * in a review is one nobody reviews.
 */
export async function migrate(pool: Pool): Promise<string[]> {
  await pool.query(`
    CREATE TABLE IF NOT EXISTS schema_migrations (
      name       text        PRIMARY KEY,
      applied_at timestamptz NOT NULL DEFAULT now()
    )`);

  const directory = join(dirname(fileURLToPath(import.meta.url)), "migrations");
  const files = (await readdir(directory)).filter((f) => f.endsWith(".sql")).sort();

  const { rows } = await pool.query<{ name: string }>("SELECT name FROM schema_migrations");
  const done = new Set(rows.map((r) => r.name));

  const applied: string[] = [];
  for (const file of files) {
    if (done.has(file)) continue;
    const sql = await readFile(join(directory, file), "utf8");
    const client = await pool.connect();
    try {
      await client.query("BEGIN");
      await client.query(sql);
      await client.query("INSERT INTO schema_migrations (name) VALUES ($1)", [file]);
      await client.query("COMMIT");
      applied.push(file);
    } catch (error) {
      await client.query("ROLLBACK");
      throw new Error(`migration ${file} failed: ${(error as Error).message}`);
    } finally {
      client.release();
    }
  }
  return applied;
}
