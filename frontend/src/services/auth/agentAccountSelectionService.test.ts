import assert from "node:assert/strict";
import test from "node:test";
import type { AgentAuthProvider } from "../../models/auth.ts";
import {
  accountsForProvider,
  resolveProviderAccountId,
  resolveRememberedAccountId,
} from "./agentAccountSelectionService.ts";

const providers: AgentAuthProvider[] = [
  provider("codex", "codex-work", ["codex-home", "codex-work"]),
  provider("claude", "claude-home", ["claude-home", "claude-work"]),
];

test("account choices belong to the selected provider", () => {
  const claudeAccounts = accountsForProvider(providers, "claude");

  assert.equal(resolveProviderAccountId(claudeAccounts, "codex-work"), "claude-home");
  assert.equal(resolveProviderAccountId(claudeAccounts, "claude-work"), "claude-work");
});

test("falls back to the first account when the advertised default is stale", () => {
  const accounts = accountsForProvider([
    provider("codex", "removed", ["personal", "work"]),
  ], "codex");

  assert.equal(resolveProviderAccountId(accounts, ""), "personal");
  assert.equal(resolveProviderAccountId(undefined, "personal"), "");
});

test("new-chat preferences preserve unknown status and repair known stale accounts", () => {
  assert.equal(resolveRememberedAccountId([], "codex", " remembered "), "remembered");
  assert.equal(resolveRememberedAccountId(providers, "claude", "codex-work"), "claude-home");
  assert.equal(resolveRememberedAccountId(providers, "claude", "claude-work"), "claude-work");
});

function provider(
  id: string,
  activeAccountId: string,
  accountIds: string[],
): AgentAuthProvider {
  return {
    provider: id,
    label: id,
    executionScopes: ["host", "project"],
    authentication: { mode: "managed-device", satisfiesAccessGate: true },
    status: {
      authenticated: true,
      login: { active: false },
      accounts: {
        activeAccountId,
        items: accountIds.map((accountId) => ({
          id: accountId,
          label: accountId,
          active: accountId === activeAccountId,
        })),
      },
    },
  };
}
