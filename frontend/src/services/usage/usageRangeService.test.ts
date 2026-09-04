import test from "node:test";
import assert from "node:assert/strict";
import { usageRangeService } from "./usageRangeService.ts";

const NOW = Date.parse("2026-08-17T15:42:11.000Z");
const DAY_MS = 24 * 60 * 60 * 1000;

/** Whole UTC days a range covers — the property the presets are defined by. */
function spanInDays(range: { from: number; to: number }): number {
  return (range.to + 1 - range.from) / DAY_MS;
}

test("bounds every range on UTC day edges", () => {
  const week = usageRangeService.forPreset("7d", NOW);
  assert.equal(new Date(week.from).toISOString(), "2026-08-11T00:00:00.000Z");
  assert.equal(new Date(week.to).toISOString(), "2026-08-17T23:59:59.999Z");
});

test("resolves each preset to an inclusive window", () => {
  const week = usageRangeService.forPreset("7d", NOW);
  assert.equal(usageRangeService.labels(week).fromDate, "2026-08-11");
  assert.equal(spanInDays(week), 7);

  const month30 = usageRangeService.forPreset("30d", NOW);
  assert.equal(usageRangeService.labels(month30).fromDate, "2026-07-19");
  assert.equal(spanInDays(month30), 30);

  const thisMonth = usageRangeService.forPreset("month", NOW);
  assert.equal(usageRangeService.labels(thisMonth).fromDate, "2026-08-01");
  assert.equal(spanInDays(thisMonth), 17);

  for (const range of [week, month30, thisMonth]) {
    // Every preset ends at the same instant: the last ms of today, UTC.
    assert.equal(new Date(range.to).toISOString(), "2026-08-17T23:59:59.999Z");
    assert.equal(usageRangeService.labels(range).toDate, "2026-08-17");
    assert.ok(range.from < range.to);
  }
});

test("this month starts on the first even when the month just rolled over", () => {
  const firstOfMonth = Date.parse("2026-09-01T00:05:00.000Z");
  const range = usageRangeService.forPreset("month", firstOfMonth);
  assert.equal(usageRangeService.labels(range).fromDate, "2026-09-01");
  assert.equal(spanInDays(range), 1);
});

test("builds a custom range and swaps reversed inputs", () => {
  const current = usageRangeService.forPreset("30d", NOW);

  const forward = usageRangeService.fromDates(current, "2026-08-01", "2026-08-03");
  assert.equal(forward.preset, "custom");
  assert.deepEqual(usageRangeService.labels(forward), { fromDate: "2026-08-01", toDate: "2026-08-03" });
  assert.equal(new Date(forward.from).toISOString(), "2026-08-01T00:00:00.000Z");
  assert.equal(spanInDays(forward), 3);

  const reversed = usageRangeService.fromDates(current, "2026-08-03", "2026-08-01");
  assert.deepEqual(usageRangeService.labels(reversed), { fromDate: "2026-08-01", toDate: "2026-08-03" });
});

test("a single custom day is one full day, not an empty window", () => {
  const range = usageRangeService.fromDates(usageRangeService.forPreset("7d", NOW), "2026-08-17", "2026-08-17");
  assert.equal(spanInDays(range), 1);
  assert.equal(range.to - range.from, DAY_MS - 1);
});

test("malformed custom input keeps the current range", () => {
  const current = usageRangeService.forPreset("7d", NOW);
  // The date-input parser's whole rejection set, exercised through the picker.
  for (const bad of ["", "17/08/2026", "2026-8-1", "not a date"]) {
    assert.deepEqual(usageRangeService.fromDates(current, bad, "2026-08-03"), current, bad);
    assert.deepEqual(usageRangeService.fromDates(current, "2026-08-03", bad), current, bad);
  }
});
