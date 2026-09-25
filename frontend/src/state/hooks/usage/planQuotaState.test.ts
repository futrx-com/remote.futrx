import assert from "node:assert/strict";
import test from "node:test";
import type { AccountQuota, QuotaWindow } from "../../../models/agentQuota.ts";
import type { AgentAuthAccount, AgentAuthProvider } from "../../../models/auth.ts";
import { projectPlanQuota, type PlanQuotaClock } from "./planQuotaState.ts";

// 2026-08-23T15:46:40Z: a fixed clock in a pinned locale and time zone, so
// reset times read the same everywhere.
const NOW = 1_787_500_000_000;
const CLOCK: PlanQuotaClock = { nowMs: NOW, locale: "en-US", timeZone: "UTC" };

function win(over: Partial<QuotaWindow> = {}): QuotaWindow {
  return { window: "session", measuredAt: NOW, ...over };
}

function reading(over: Partial<AccountQuota> = {}): AccountQuota {
  return { provider: "claude", accountId: "", session: win(), ...over };
}

function authProvider(
  provider: string,
  label: string,
  accounts?: AgentAuthAccount[],
): AgentAuthProvider {
  return {
    provider,
    label,
    executionScopes: ["host", "project"],
    authentication: { mode: "managed-device", satisfiesAccessGate: true },
    status: {
      authenticated: true,
      login: { active: false },
      accounts: accounts && {
        activeAccountId: accounts.find((account) => account.active)?.id,
        items: accounts,
      },
    },
  };
}

/** One window of the host-login plan of a provider without saved accounts. */
function project(session: QuotaWindow, provider = "claude", clock = CLOCK) {
  return projectPlanQuota([reading({ provider, session })], [], clock)[0].accounts[0].windows[0];
}

test("each provider's windows read the way its own CLI prints them", () => {
  // Claude Code's /usage counts what is used; Codex's /status what is left.
  const claude = projectPlanQuota([reading({
    session: win({ usedPercent: 24.9 }),
    weekly: win({ window: "weekly", usedPercent: 61 }),
  })], [], CLOCK)[0].accounts[0].windows;
  assert.deepEqual(
    claude.map((window) => [window.label, window.value, window.barPercent]),
    [["Current session", "24% used", 24], ["Current week (all models)", "61% used", 61]],
  );

  const codex = projectPlanQuota([reading({
    provider: "codex",
    session: win({ usedPercent: 45.4 }),
    weekly: win({ window: "weekly", usedPercent: 108 }),
  })], [], CLOCK)[0].accounts[0].windows;
  assert.deepEqual(
    codex.map((window) => [window.label, window.value, window.barPercent]),
    [["5h limit", "55% left", 55], ["Weekly limit", "0% left", 0]],
  );

  const minimax = project(win({ usedPercent: 35 }), "minimax");
  assert.deepEqual([minimax.label, minimax.value, minimax.barPercent], ["5h limit", "65% left", 65]);

  const other = project(win({ usedPercent: 12 }), "future-cli");
  assert.deepEqual([other.label, other.value], ["5-hour limit", "12% used"]);
});

test("a window with no number is not drawn as zero used", () => {
  // A run's Claude status reading has no percentage. Treating that as 0% would
  // tell the operator their plan is untouched, which the CLI never said.
  assert.deepEqual([project(win({ status: "allowed" })).tone, project(win({ status: "allowed" })).value], ["ok", "fine"]);
  assert.equal(project(win({ status: "allowed_warning" })).value, "getting low");
  assert.equal(project(win({})).tone, "unknown");
  assert.equal(project(win({})).value, "not reported");
  assert.equal(project(win({})).barPercent, null);
});

test("a rejected window reads as spent whatever the percentage says", () => {
  // The vendor refusing is the fact; a stale percentage is not.
  assert.equal(project(win({ status: "rejected", usedPercent: 10 })).tone, "spent");
});

test("the used percentage sets the tone in either measure", () => {
  for (const provider of ["claude", "codex", "minimax"]) {
    assert.equal(project(win({ usedPercent: 12 }), provider).tone, "ok");
    assert.equal(project(win({ usedPercent: 74 }), provider).tone, "warn");
    assert.equal(project(win({ usedPercent: 95 }), provider).tone, "spent");
  }
});

test("zero usage has zero width and over-limit usage fills the bar", () => {
  assert.deepEqual(
    [0, 12.5, 120].map((usedPercent) => {
      const window = project(win({ usedPercent }));
      return [window.value, window.barPercent];
    }),
    [["0% used", 0], ["12% used", 12], ["120% used", 100]],
  );
});

test("invalid percentages remain unknown and never produce a numeric bar", () => {
  for (const usedPercent of [NaN, Infinity, -Infinity, -1]) {
    const window = project(win({ usedPercent }));
    assert.equal(window.barPercent, null);
    assert.equal(window.tone, "unknown");
    assert.equal(project(win({ usedPercent, status: "allowed_warning" })).tone, "warn");
  }
});

test("reset times read like the CLIs, with the countdown kept for a tooltip", () => {
  const today = project(win({ resetsAt: NOW / 1000 + 9000 }));
  assert.deepEqual([today.reset, today.resetIn], ["resets 6:16 PM", "in 2h 30m"]);
  const later = project(win({ resetsAt: NOW / 1000 + 200000 }));
  assert.deepEqual([later.reset, later.resetIn], ["resets Aug 25, 11:20 PM", "in 2d 7h"]);
  const passed = project(win({ resetsAt: NOW / 1000 - 5 }));
  assert.deepEqual([passed.reset, passed.resetIn], ["reset passed; awaiting a new reading", ""]);
  // A reading may omit its reset time.
  assert.deepEqual([project(win({})).reset, project(win({})).resetIn], ["", ""]);
  // The day boundary follows the viewer's time zone: past midnight in UTC,
  // still the same evening in Los Angeles.
  const overnight = win({ resetsAt: NOW / 1000 + 30000 });
  assert.equal(project(overnight).reset, "resets Aug 24, 12:06 AM");
  assert.equal(project(overnight, "claude", { ...CLOCK, timeZone: "America/Los_Angeles" }).reset, "resets 5:06 PM");
});

test("a current reading says nothing about its age and a stale one does", () => {
  assert.equal(project(win({ measuredAt: NOW - 60_000 })).updated, "");
  assert.equal(project(win({ measuredAt: NOW - 5 * 60_000 })).updated, "updated 5m ago");
  assert.equal(project(win({ measuredAt: NOW - 3 * 3_600_000 })).updated, "updated 3h ago");
  assert.equal(project(win({ measuredAt: NOW - 50 * 3_600_000 })).updated, "updated 2d ago");
  for (const invalid of [0, -1, NaN, Infinity, -Infinity]) {
    const window = project(win({ measuredAt: invalid, resetsAt: invalid }));
    assert.equal(window.updated, "updated at an unknown time");
    assert.equal(window.reset, "");
  }
});

test("ages advance with the display clock", () => {
  const quotas = [reading({ session: win({ measuredAt: NOW - 4 * 60_000, resetsAt: NOW / 1000 + 120 }) })];
  const initial = projectPlanQuota(quotas, [], CLOCK)[0].accounts[0].windows[0];
  const later = projectPlanQuota(quotas, [], { ...CLOCK, nowMs: NOW + 60_000 })[0].accounts[0].windows[0];
  assert.deepEqual([initial.updated, initial.resetIn], ["", "in 2m"]);
  assert.deepEqual([later.updated, later.resetIn], ["updated 5m ago", "in 1m"]);
});

test("an account with no window is not rendered unless its read failed", () => {
  assert.deepEqual(projectPlanQuota([reading({ session: undefined })], [], CLOCK), []);

  const catalog = [authProvider("codex", "Codex", [{ id: "work", label: "Work", active: true }])];
  const [codex] = projectPlanQuota([
    reading({ provider: "codex", accountId: "work", session: undefined, error: "sign-in expired" }),
  ], catalog, CLOCK);
  assert.deepEqual(
    codex.accounts.map((account) => [account.id, account.error, account.windows.length]),
    [["work", "sign-in expired", 0]],
  );
});

test("a failed read keeps the last windows beside its explanation", () => {
  const [claude] = projectPlanQuota([
    reading({ session: win({ usedPercent: 30, measuredAt: NOW - 2 * 3_600_000 }), error: "offline" }),
  ], [], CLOCK);
  assert.deepEqual(
    [claude.accounts[0].error, claude.accounts[0].windows[0].value, claude.accounts[0].windows[0].updated],
    ["offline", "30% used", "updated 2h ago"],
  );
});

test("each saved account shows its own plan under its catalog label", () => {
  // Two subscriptions of one provider are two plans; blending them would put
  // one account's nearly spent week next to the other's name.
  const catalog = [authProvider("codex", "Codex", [
    { id: "work", label: "Work", email: "me@work.test", planType: "pro", active: true },
    { id: "personal", label: "Personal", active: false },
  ])];
  const quotas = [
    reading({ provider: "codex", accountId: "personal", session: win({ usedPercent: 5 }) }),
    reading({ provider: "codex", accountId: "work", session: win({ usedPercent: 95 }) }),
  ];

  const [codex] = projectPlanQuota(quotas, catalog, CLOCK);
  assert.equal(codex.label, "Codex");
  assert.deepEqual(
    codex.accounts.map((account) => [account.id, account.label, account.detail, account.active, account.windows[0].value]),
    [
      ["work", "Work", "me@work.test · pro", true, "5% left"],
      ["personal", "Personal", "", false, "95% left"],
    ],
  );
});

test("an active saved account hides readings no saved account owns", () => {
  // Chats now run on saved accounts, so the current login's reading is from
  // before, and a removed account's plan belongs to no one; showing either
  // would misattribute a real number.
  const catalog = [authProvider("claude", "Claude", [{ id: "work", label: "Work", active: true }])];
  const quotas = [
    reading({ accountId: "", session: win({ usedPercent: 40 }) }),
    reading({ accountId: "removed", session: win({ usedPercent: 80 }) }),
    reading({ accountId: "work", session: win({ usedPercent: 10 }) }),
  ];
  const projected = projectPlanQuota(quotas, catalog, CLOCK);
  assert.deepEqual(projected[0].accounts.map((account) => account.id), ["work"]);

  const onlyUnowned = quotas.filter((quota) => quota.accountId !== "work");
  assert.deepEqual(projectPlanQuota(onlyUnowned, catalog, CLOCK), []);
});

test("the current login's plan stays listed while no saved account is active", () => {
  // A login saved during a run is not activated, so chats without a pinned
  // account keep running on the current login until an account is activated.
  const catalog = [authProvider("claude", "Claude", [{ id: "new", label: "New", active: false }])];
  const quotas = [
    reading({ accountId: "new", session: win({ usedPercent: 10 }) }),
    reading({ accountId: "", session: win({ usedPercent: 40 }) }),
    reading({ accountId: "removed", session: win({ usedPercent: 80 }) }),
  ];
  const [claude] = projectPlanQuota(quotas, catalog, CLOCK);
  assert.deepEqual(
    claude.accounts.map((account) => [account.id, account.label, account.active, account.windows[0].value]),
    [
      ["", "Current login", false, "40% used"],
      ["new", "New", false, "10% used"],
    ],
  );
});

test("a provider without saved accounts shows its current-login plan under the provider", () => {
  for (const catalog of [[authProvider("kimi", "Kimi")], [authProvider("kimi", "Kimi", [])], []]) {
    const quotas = [
      reading({ provider: "kimi", accountId: "", session: win({ usedPercent: 30 }) }),
      reading({ provider: "kimi", accountId: "stale", session: win({ usedPercent: 90 }) }),
    ];
    const [kimi] = projectPlanQuota(quotas, catalog, CLOCK);
    assert.equal(kimi.label, "Kimi");
    assert.deepEqual(
      kimi.accounts.map((account) => [account.id, account.label, account.active, account.windows[0].value]),
      [["", "", false, "30% used"]],
    );
  }
});

test("providers follow the catalog order and uncataloged ones follow by id", () => {
  const catalog = [
    authProvider("codex", "Codex"),
    authProvider("minimax", "MiniMax", [{ id: "key", label: "Token Plan", active: true }]),
    authProvider("claude", "Claude"),
  ];
  const quotas = [
    reading({ provider: "future-cli" }),
    reading({ provider: "claude" }),
    reading({ provider: "codex" }),
    reading({ provider: "another-cli" }),
  ];
  assert.deepEqual(
    projectPlanQuota(quotas, catalog, CLOCK).map((provider) => provider.label),
    ["Codex", "Claude", "Another Cli", "Future Cli"],
  );
});
