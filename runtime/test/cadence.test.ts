/** The translation between what the app sends and what the timer reads. */

import assert from "node:assert/strict";
import { test } from "node:test";

import { cadenceOf, cronOf, describe, validCadence } from "../src/cadence.ts";

const TZ = "Asia/Kolkata";

test("each repeat becomes the cron line the timer reads", () => {
  assert.equal(cronOf({ repeat: "daily", time: "07:00", timezone: TZ }), "0 7 * * *");
  assert.equal(cronOf({ repeat: "weekdays", time: "06:30", timezone: TZ }), "30 6 * * 1-5");
  assert.equal(cronOf({ repeat: "weekly", time: "18:05", days: ["mon", "fri"], timezone: TZ }), "5 18 * * 1,5");
  assert.equal(cronOf({ repeat: "once", time: "09:15", date: "2026-09-12", timezone: TZ }), "15 9 12 9 *");
});

test("routines set before the app could see them are read back, when they can be", () => {
  assert.deepEqual(cadenceOf("0 8 * * *", TZ), { repeat: "daily", time: "08:00", timezone: TZ });
  assert.deepEqual(cadenceOf("30 18 * * 1-5", TZ), { repeat: "weekdays", time: "18:30", timezone: TZ });
  assert.deepEqual(cadenceOf("0 9 * * 5", TZ), { repeat: "weekly", time: "09:00", days: ["fri"], timezone: TZ });
  // Every eight minutes is not something the app can show, and stays put.
  assert.equal(cadenceOf("*/8 * * * *", TZ), undefined);
});

test("the time is said back in words", () => {
  assert.equal(describe({ repeat: "daily", time: "07:00", timezone: TZ }), "every day at 7:00 am");
  assert.equal(describe({ repeat: "weekdays", time: "18:30", timezone: TZ }), "every weekday at 6:30 pm");
  assert.equal(describe({ repeat: "once", time: "00:05", date: "2026-09-12", timezone: TZ }), "on 2026-09-12 at 12:05 am");
});

test("only what can be held is valid", () => {
  assert.equal(validCadence({ repeat: "daily", time: "07:00", timezone: TZ }), true);
  assert.equal(validCadence({ repeat: "daily", time: "7:00", timezone: TZ }), false);
  assert.equal(validCadence({ repeat: "weekly", time: "07:00", timezone: TZ }), false);
  assert.equal(validCadence({ repeat: "once", time: "07:00", timezone: TZ }), false);
});
