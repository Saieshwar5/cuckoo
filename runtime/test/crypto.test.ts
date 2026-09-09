/**
 * Encryption for what this database must hold but must not leak.
 */

import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { test } from "node:test";

import { decrypt, encrypt } from "../src/crypto.ts";

const KEY = randomBytes(32);
const SECRET = "bnd_sec_0123456789abcdefghjkmnpqrstvwxyz";

test("what was encrypted comes back", () => {
  assert.equal(decrypt(KEY, encrypt(KEY, SECRET)), SECRET);
});

test("the same secret encrypts differently every time", () => {
  // A fresh nonce each time, so two agents holding the same value do not
  // produce the same row — which would say so to anyone reading the table.
  assert.notEqual(encrypt(KEY, SECRET), encrypt(KEY, SECRET));
});

test("a stolen row without the key is nothing", () => {
  const stored = encrypt(KEY, SECRET);
  assert.equal(stored.includes(SECRET), false);
  assert.throws(() => decrypt(randomBytes(32), stored));
});

test("a tampered row fails rather than decrypting to rubbish", () => {
  const stored = encrypt(KEY, SECRET);
  const raw = Buffer.from(stored.slice(3), "base64");
  // Flip a bit in the ciphertext. GCM's tag is what notices.
  raw.writeUInt8(raw.readUInt8(raw.length - 1) ^ 0xff, raw.length - 1);
  assert.throws(() => decrypt(KEY, `v1:${raw.toString("base64")}`));
});

test("something this did not write is refused", () => {
  assert.throws(() => decrypt(KEY, "not-ours"), /not something encrypt/);
});
