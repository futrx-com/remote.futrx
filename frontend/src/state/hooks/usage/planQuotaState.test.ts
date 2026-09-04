import assert from "node:assert/strict";
import test from "node:test";
import type { AgentQuota, QuotaWindow } from "../../../models/agentQuota.ts";
import { projectPlanQuotaRows } from "./planQuotaState.ts";

const NOW = 1_787_500_000_000; // fixed clock: these are all relative readings

function win(over: Partial<QuotaWindow> = {}): QuotaWindow {
  return { window: "session", measuredAt: NOW, ...over };
}

function project(session: QuotaWindow) {
  return projectPlanQuotaRows([{ provider: "claude", session }], NOW)[0];
}

test("a window with no number is not drawn as zero used", () => {
  // Claude reports a status and no percentage. Treating that as 0% would tell
  // the operator their plan is untouched, which the CLI never said.
  assert.equal(project(win({ status: "allowed" })).windows[0].tone, "ok");
  assert.equal(project(win({ status: "allowed_warning" })).windows[0].tone, "warn");
  assert.equal(project(win({})).windows[0].tone, "unknown");
  assert.equal(project(win({})).windows[0].percent, null);
});

test("a rejected window reads as spent whatever the percentage says", () => {
  // The vendor refusing is the fact; a stale percentage is not.
  assert.equal(
    project(win({ status: "rejected", usedPercent: 10 })).windows[0].tone,
    "spent"
  );
});

test("percentages set the tone when the CLI sends them", () => {
  assert.equal(project(win({ usedPercent: 12 })).windows[0].tone, "ok");
  assert.equal(project(win({ usedPercent: 74 })).windows[0].tone, "warn");
  assert.equal(project(win({ usedPercent: 95 })).windows[0].tone, "spent");
});

test("an agent with no reported window is not rendered", () => {
  assert.deepEqual(projectPlanQuotaRows([{ provider: "claude" }], NOW), []);
});

test("the countdown is human and stops at zero", () => {
  assert.equal(
    project(win({ resetsAt: NOW / 1000 + 9000 })).windows[0].reset,
    "resets in 2h 30m"
  );
  assert.equal(
    project(win({ resetsAt: NOW / 1000 + 600 })).windows[0].reset,
    "resets in 10m"
  );
  assert.equal(
    project(win({ resetsAt: NOW / 1000 + 200000 })).windows[0].reset,
    "resets in 2d 7h"
  );
  assert.equal(
    project(win({ resetsAt: NOW / 1000 - 5 })).windows[0].reset,
    "resets any moment"
  );
  // No reset time is normal on codex, which reports a window length instead.
  assert.equal(project(win({})).windows[0].reset, "");
});

test("every reading says how old it is", () => {
  // The caveat is the point: readings only arrive during a run, so an idle
  // platform is showing a number from whenever it last worked.
  assert.equal(project(win({ measuredAt: NOW })).measured, "just now");
  assert.equal(project(win({ measuredAt: NOW - 5 * 60000 })).measured, "5m ago");
  assert.equal(project(win({ measuredAt: NOW - 3 * 3600000 })).measured, "3h ago");
  assert.equal(project(win({ measuredAt: NOW - 50 * 3600000 })).measured, "2d ago");
});

test("agents are named the way the operator names them", () => {
  const quotas: AgentQuota[] = [
    { provider: "claude", session: win() },
    { provider: "codex", session: win() },
    { provider: "future-cli", session: win() },
  ];
  const labels = projectPlanQuotaRows(quotas, NOW).map((row) => row.label);
  assert.deepEqual(labels, ["Claude", "Codex", "future-cli"]);
});

test("percentages keep their display rounding and visible bar bounds", () => {
  assert.deepEqual(
    [0, 12.5, 120].map((usedPercent) => {
      const window = project(win({ usedPercent })).windows[0];
      return [window.percent, window.barPercent];
    }),
    [
      [0, 2],
      [13, 13],
      [120, 100],
    ]
  );
});
