import assert from "node:assert/strict";
import test from "node:test";
import type { AgentAuthProvider } from "../../models/auth.ts";
import { agentAuthRegistryService } from "./agentAuthRegistryService.ts";

function provider(
  id: string,
  mode: AgentAuthProvider["authentication"]["mode"],
  authenticated: boolean,
  satisfiesAccessGate = false,
): AgentAuthProvider {
  return {
    provider: id,
    label: id === "future-agent" ? "Future Agent" : id === "minimax" ? "MiniMax" : id,
    executionScopes: ["host"],
    authentication: { mode, satisfiesAccessGate },
    status: { authenticated, login: { active: false } },
  };
}

test("an arbitrary managed module participates in gating and availability", () => {
  const providers = [
    provider("external-agent", "external", false),
    provider("future-agent", "managed-device", false, true),
  ];
  assert.equal(agentAuthRegistryService.gateReady(providers), false);
  assert.deepEqual(agentAuthRegistryService.unavailableReasons(providers, true), {
    "future-agent": "Log in to Future Agent in Settings before selecting it.",
  });

  const updated = agentAuthRegistryService.updateProvider(providers, "future-agent", {
    authenticated: true,
    login: { active: false, completed: true, startedAt: 42 },
  });
  assert.equal(agentAuthRegistryService.gateReady(updated), true);
  assert.equal(providers[1].status.authenticated, false);
  assert.notEqual(agentAuthRegistryService.revision(updated), agentAuthRegistryService.revision(providers));
});

test("external modules neither block the managed gate nor receive login errors", () => {
  const providers = [provider("external-agent", "external", false)];
  assert.equal(agentAuthRegistryService.gateReady(providers), false);
  assert.deepEqual(agentAuthRegistryService.unavailableReasons(providers, true), {});
});

test("no-auth gate modules are immediately ready from the backend snapshot", () => {
  const local = provider("local-agent", "none", true, true);
  assert.equal(agentAuthRegistryService.gateReady([local]), true);
  assert.equal(agentAuthRegistryService.statusKind(local), "no-auth");
});

test("auth display state follows the declared flow before generic status", () => {
  assert.equal(agentAuthRegistryService.statusKind(provider("future-agent", "managed-device", true)), "authenticated");
  assert.equal(agentAuthRegistryService.statusKind(provider("external-agent", "external", false)), "external");
  assert.equal(agentAuthRegistryService.statusKind(provider("future-agent", "managed-code", false)), "unconfigured");
  assert.equal(agentAuthRegistryService.statusKind(provider("minimax", "managed-api-key", false)), "unconfigured");
  assert.deepEqual(
    agentAuthRegistryService.unavailableReasons(
      [provider("minimax", "managed-api-key", false)],
      true,
    ),
    { minimax: "Sign in to MiniMax in Settings → Agents, then refresh models." },
  );
});

test("switching the active account invalidates provider capability state", () => {
  const codex = provider("codex", "managed-device", true, true);
  codex.status.accounts = {
    activeAccountId: "personal",
    items: [{ id: "personal", label: "Personal", active: true }],
  };
  const before = agentAuthRegistryService.revision([codex]);
  const switched: AgentAuthProvider = {
    ...codex,
    status: {
      ...codex.status,
      accounts: {
        activeAccountId: "work",
        items: [{ id: "work", label: "Work", active: true }],
      },
    },
  };
  assert.notEqual(agentAuthRegistryService.revision([switched]), before);
});
