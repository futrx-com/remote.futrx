import assert from "node:assert/strict";
import test from "node:test";
import type { AccountQuota, QuotaWindow } from "../../../models/agentQuota.ts";
import type { AgentAuthAccount, AgentAuthProvider } from "../../../models/auth.ts";
import { projectPlanQuota } from "./planQuotaState.ts";

const NOW = 1_787_500_000_000; // fixed clock: these are all relative readings

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

/** The host-login plan of a provider without saved accounts. */
function project(session: QuotaWindow) {
  return projectPlanQuota([reading({ session })], [], NOW)[0].accounts[0];
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

test("an account with no reported window is not rendered", () => {
  assert.deepEqual(projectPlanQuota([reading({ session: undefined })], [], NOW), []);
});

test("the countdown labels an expired snapshot instead of promising an imminent reset", () => {
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
    "reset passed; awaiting a new reading"
  );
  // A reading may omit its reset time.
  assert.equal(project(win({})).windows[0].reset, "");
});

test("every reading says how old it is", () => {
  // The caveat is the point: readings only arrive during a run, so an idle
  // platform is showing a number from whenever it last worked.
  assert.equal(project(win({ measuredAt: NOW })).windows[0].measured, "just now");
  assert.equal(project(win({ measuredAt: NOW - 5 * 60000 })).windows[0].measured, "5m ago");
  assert.equal(project(win({ measuredAt: NOW - 3 * 3600000 })).windows[0].measured, "3h ago");
  assert.equal(project(win({ measuredAt: NOW - 50 * 3600000 })).windows[0].measured, "2d ago");
});

test("zero usage has zero width and percentages retain rounding and the bar maximum", () => {
  assert.deepEqual(
    [0, 12.5, 120].map((usedPercent) => {
      const window = project(win({ usedPercent })).windows[0];
      return [window.percent, window.barPercent];
    }),
    [
      [0, 0],
      [13, 13],
      [120, 100],
    ]
  );
});

test("invalid percentages remain unknown and never produce a numeric bar", () => {
  for (const usedPercent of [NaN, Infinity, -Infinity, -1]) {
    const window = project(win({ usedPercent })).windows[0];
    assert.equal(window.percent, null);
    assert.equal(window.barPercent, null);
    assert.equal(window.tone, "unknown");
    assert.equal(project(win({ usedPercent, status: "allowed_warning" })).windows[0].tone, "warn");
  }
});

test("each window keeps its own observation age and advances with the display clock", () => {
  const quotas = [reading({
    session: win({ measuredAt: NOW - 10 * 60000 }),
    weekly: win({ window: "weekly", measuredAt: NOW, resetsAt: NOW / 1000 + 120 }),
  })];
  const initial = projectPlanQuota(quotas, [], NOW)[0].accounts[0].windows;
  assert.deepEqual(initial.map((window) => [window.kind, window.measured]), [
    ["session", "10m ago"],
    ["weekly", "just now"],
  ]);
  const later = projectPlanQuota(quotas, [], NOW + 60000)[0].accounts[0].windows;
  assert.deepEqual(later.map((window) => window.measured), ["11m ago", "1m ago"]);
  assert.equal(initial[1].reset, "resets in 2m");
  assert.equal(later[1].reset, "resets in 1m");
});

test("invalid timestamps do not claim a fresh reading or render a broken countdown", () => {
  for (const invalid of [0, -1, NaN, Infinity, -Infinity]) {
    const window = project(win({ measuredAt: invalid, resetsAt: invalid })).windows[0];
    assert.equal(window.measured, "at an unknown time");
    assert.equal(window.reset, "");
  }
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

  const [codex] = projectPlanQuota(quotas, catalog, NOW);
  assert.equal(codex.label, "Codex");
  assert.deepEqual(
    codex.accounts.map((account) => [account.id, account.label, account.detail, account.active, account.windows[0].percent]),
    [
      ["work", "Work", "me@work.test · pro", true, 95],
      ["personal", "Personal", "", false, 5],
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
  const projected = projectPlanQuota(quotas, catalog, NOW);
  assert.deepEqual(projected[0].accounts.map((account) => account.id), ["work"]);

  const onlyUnowned = quotas.filter((quota) => quota.accountId !== "work");
  assert.deepEqual(projectPlanQuota(onlyUnowned, catalog, NOW), []);
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
  const [claude] = projectPlanQuota(quotas, catalog, NOW);
  assert.deepEqual(
    claude.accounts.map((account) => [account.id, account.label, account.active, account.windows[0].percent]),
    [
      ["", "Current login", false, 40],
      ["new", "New", false, 10],
    ],
  );
});

test("a provider without saved accounts shows its current-login plan under the provider", () => {
  for (const catalog of [[authProvider("kimi", "Kimi")], [authProvider("kimi", "Kimi", [])], []]) {
    const quotas = [
      reading({ provider: "kimi", accountId: "", session: win({ usedPercent: 30 }) }),
      reading({ provider: "kimi", accountId: "stale", session: win({ usedPercent: 90 }) }),
    ];
    const [kimi] = projectPlanQuota(quotas, catalog, NOW);
    assert.equal(kimi.label, "Kimi");
    assert.deepEqual(
      kimi.accounts.map((account) => [account.id, account.label, account.active, account.windows[0].percent]),
      [["", "", false, 30]],
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
    projectPlanQuota(quotas, catalog, NOW).map((provider) => provider.label),
    ["Codex", "Claude", "Another Cli", "Future Cli"],
  );
});
