/**
 * Encryption for the things this database must hold but must not leak: the
 * binding secrets that speak for agents, and later the app tokens that reach
 * into someone's mail.
 *
 * AES-256-GCM with a key that lives in the environment and never in a row, so
 * a stolen dump — or a stolen backup — is ciphertext. The key id travels with
 * the ciphertext so a key can be rotated by re-encrypting in the background
 * rather than by taking everything down at once.
 */

import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";

const IV_BYTES = 12; // what GCM is defined for
const TAG_BYTES = 16;

/**
 * Encrypt with the current key. The result is `v1:<base64>` where the bytes
 * are iv ‖ tag ‖ ciphertext — self-contained, so decrypting needs nothing but
 * the key.
 */
export function encrypt(key: Buffer, plaintext: string): string {
  const iv = randomBytes(IV_BYTES);
  const cipher = createCipheriv("aes-256-gcm", key, iv);
  const body = Buffer.concat([cipher.update(plaintext, "utf8"), cipher.final()]);
  return `v1:${Buffer.concat([iv, cipher.getAuthTag(), body]).toString("base64")}`;
}

export function decrypt(key: Buffer, stored: string): string {
  const [version, payload] = stored.split(":", 2);
  if (version !== "v1" || !payload) throw new Error("this is not something encrypt() wrote");
  const raw = Buffer.from(payload, "base64");
  const iv = raw.subarray(0, IV_BYTES);
  const tag = raw.subarray(IV_BYTES, IV_BYTES + TAG_BYTES);
  const body = raw.subarray(IV_BYTES + TAG_BYTES);
  const decipher = createDecipheriv("aes-256-gcm", key, iv);
  decipher.setAuthTag(tag);
  // A wrong key, or a tampered row, fails here rather than returning rubbish.
  return Buffer.concat([decipher.update(body), decipher.final()]).toString("utf8");
}
