import assert from "node:assert/strict";
import test from "node:test";
import { limitResetText, type LimitResetClock } from "./limitResetText.ts";

// 2026-09-10 15:00 UTC, the day the issue was filed.
const sent = Date.UTC(2026, 8, 10, 15, 0);

function clock(timeZone: string, overrides: Partial<LimitResetClock> = {}): LimitResetClock {
  return { sentMs: sent, nowMs: sent, locale: "en-US", timeZone, ...overrides };
}

test("rewrites the reset time in the viewer's time zone", () => {
  assert.equal(
    limitResetText.localize("You've hit your session limit · resets 7:50pm (UTC)", clock("Asia/Dubai")),
    "You've hit your session limit · resets 11:50 PM (GMT+4)",
  );
});

test("adds the date when the reset falls on another day for the viewer", () => {
  // 14:00 UTC is 23:00 on Sep 10 in Tokyo; the 19:50 UTC reset is 04:50 on Sep 11 there.
  const evening = Date.UTC(2026, 8, 10, 14, 0);
  assert.equal(
    limitResetText.localize(
      "You've hit your session limit · resets 7:50pm (UTC)",
      clock("Asia/Tokyo", { sentMs: evening, nowMs: evening }),
    ),
    "You've hit your session limit · resets Sep 11, 4:50 AM (GMT+9)",
  );
});

test("takes the next occurrence after the message was sent", () => {
  // Sent at 20:00 UTC, "resets 7:50pm" is the next evening, not ten minutes ago.
  const late = Date.UTC(2026, 8, 10, 20, 0);
  assert.equal(
    limitResetText.localize("resets 7:50pm (UTC)", clock("UTC", { sentMs: late, nowMs: late })),
    "resets Sep 11, 7:50 PM (UTC)",
  );
});

test("anchors an old message to when it was sent, not to now", () => {
  const nextWeek = sent + 7 * 24 * 60 * 60 * 1000;
  assert.equal(
    limitResetText.localize("resets 7:50pm (UTC)", clock("UTC", { nowMs: nextWeek })),
    "resets Sep 10, 7:50 PM (UTC)",
  );
});

test("reads hour-only times and dated resets", () => {
  assert.equal(limitResetText.localize("resets 7pm (UTC)", clock("UTC")), "resets 7:00 PM (UTC)");
  assert.equal(
    limitResetText.localize("Weekly limit reached · resets Oct 3, 5pm (UTC)", clock("Asia/Dubai")),
    "Weekly limit reached · resets Oct 3, 9:00 PM (GMT+4)",
  );
  assert.equal(
    limitResetText.localize("resets Oct 3 at 5:30pm (UTC)", clock("UTC")),
    "resets Oct 3, 5:30 PM (UTC)",
  );
});

test("a dated reset early in the year that has already passed means next year", () => {
  const december = Date.UTC(2026, 11, 30, 12, 0);
  assert.equal(
    limitResetText.localize("resets Jan 2, 5pm (UTC)", clock("UTC", { sentMs: december, nowMs: december })),
    "resets Jan 2, 5:00 PM (UTC)",
  );
});

test("leaves other text and malformed times unchanged", () => {
  const cases = [
    "Rate limited, try again later",
    "resets 7:50pm (America/New_York)",
    "resets 13:50pm (UTC)",
    "resets Feb 31, 5pm (UTC)",
    "resets Foo 3, 5pm (UTC)",
  ];
  for (const message of cases) {
    assert.equal(limitResetText.localize(message, clock("Asia/Dubai")), message);
  }
});
