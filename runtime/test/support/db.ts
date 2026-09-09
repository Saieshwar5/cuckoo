/**
 * A migrated database for the tests that need one.
 *
 * The schema is built from empty every run, because a migration that only
 * works against a database that already has the table is one that fails on the
 * first real deploy and nowhere else.
 */

import pg from "pg";

import { migrate, openPool, type Pool } from "../../src/db/pool.ts";

const ADMIN_URL =
  process.env.RUNTIME_TEST_ADMIN_URL ?? "postgres://cuckoo:cuckoo@localhost:5433/postgres";
const BASE = process.env.RUNTIME_TEST_DB ?? "cuckoo_runtime_test";

function urlFor(name: string): string {
  return `postgres://cuckoo:cuckoo@localhost:5433/${name}?sslmode=disable`;
}

/** True when a Postgres is there to test against. */
export async function databaseAvailable(): Promise<boolean> {
  const client = new pg.Client({ connectionString: ADMIN_URL, connectionTimeoutMillis: 1500 });
  try {
    await client.connect();
    await client.end();
    return true;
  } catch {
    return false;
  }
}

/**
 * Drop, create and migrate a test database, and hand back a pool on it.
 *
 * `suffix` gives each test file its own: node runs the files in parallel, and
 * two of them dropping the same database is a deadlock rather than a failure,
 * which is a much worse afternoon.
 */
export async function freshPool(suffix: string): Promise<Pool> {
  const name = `${BASE}_${suffix}`;
  const admin = new pg.Client({ connectionString: ADMIN_URL });
  await admin.connect();
  await admin.query(`DROP DATABASE IF EXISTS ${name} WITH (FORCE)`);
  await admin.query(`CREATE DATABASE ${name} OWNER cuckoo`);
  await admin.end();

  const pool = openPool(urlFor(name));
  await migrate(pool);
  return pool;
}
