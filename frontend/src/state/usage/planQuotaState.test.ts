import assert from "node:assert/strict";
import test from "node:test";
import {
  agentQuotaLabel,
  measuredAgo,
  quotaTone,
  resetsIn,
  type QuotaWindow,
} from "../../models/agentQuota.ts";

const NOW = 1_787_500_000_000; // fixed clock: these are all relative readings

function win(over: Partial<QuotaWindow> = {}): QuotaWindow {
  return { window: "session", measuredAt: NOW, ...over };
}

test("a window with no number is not drawn as zero used", () => {
  // Claude reports a status and no percentage. Treating that as 0% would tell
  // the operator their plan is untouched, which the CLI never said.
  assert.equal(quotaTone(win({ status: "allowed" })), "ok");
  assert.equal(quotaTone(win({ status: "allowed_warning" })), "warn");
  assert.equal(quotaTone(win({})), "unknown");
});

test("a rejected window reads as spent whatever the percentage says", () => {
  // The vendor refusing is the fact; a stale percentage is not.
  assert.equal(quotaTone(win({ status: "rejected", usedPercent: 10 })), "spent");
});

test("percentages set the tone when the CLI sends them", () => {
  assert.equal(quotaTone(win({ usedPercent: 12 })), "ok");
  assert.equal(quotaTone(win({ usedPercent: 74 })), "warn");
  assert.equal(quotaTone(win({ usedPercent: 95 })), "spent");
});

test("an absent window is unknown, not fine", () => {
  assert.equal(quotaTone(undefined), "unknown");
});

test("the countdown is human and stops at zero", () => {
  assert.equal(resetsIn(win({ resetsAt: NOW / 1000 + 9000 }), NOW), "resets in 2h 30m");
  assert.equal(resetsIn(win({ resetsAt: NOW / 1000 + 600 }), NOW), "resets in 10m");
  assert.equal(resetsIn(win({ resetsAt: NOW / 1000 + 200000 }), NOW), "resets in 2d 7h");
  assert.equal(resetsIn(win({ resetsAt: NOW / 1000 - 5 }), NOW), "resets any moment");
  // No reset time is normal on codex, which reports a window length instead.
  assert.equal(resetsIn(win({}), NOW), "");
});

test("every reading says how old it is", () => {
  // The caveat is the point: readings only arrive during a run, so an idle
  // platform is showing a number from whenever it last worked.
  assert.equal(measuredAgo(win({ measuredAt: NOW }), NOW), "just now");
  assert.equal(measuredAgo(win({ measuredAt: NOW - 5 * 60000 }), NOW), "5m ago");
  assert.equal(measuredAgo(win({ measuredAt: NOW - 3 * 3600000 }), NOW), "3h ago");
  assert.equal(measuredAgo(win({ measuredAt: NOW - 50 * 3600000 }), NOW), "2d ago");
});

test("agents are named the way the operator names them", () => {
  assert.equal(agentQuotaLabel("claude"), "Claude");
  assert.equal(agentQuotaLabel("codex"), "Codex");
  // An agent the UI has no label for still renders as something.
  assert.equal(agentQuotaLabel("future-cli"), "future-cli");
});
