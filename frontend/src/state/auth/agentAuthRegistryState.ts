import type { AgentAuthProvider, AgentAuthSnapshot } from "../../models/auth";

export type AgentAuthStatusKind = "no-auth" | "authenticated" | "external" | "unconfigured";

export function agentAuthStatusKind(entry: AgentAuthProvider): AgentAuthStatusKind {
  if (entry.authentication.mode === "none") return "no-auth";
  if (entry.status.authenticated) return "authenticated";
  if (entry.authentication.mode === "external") return "external";
  return "unconfigured";
}

export function updateAgentAuthProvider(
  providers: AgentAuthProvider[],
  provider: string,
  status: AgentAuthSnapshot,
): AgentAuthProvider[] {
  return providers.map((entry) =>
    entry.provider === provider ? { ...entry, status } : entry
  );
}

export function agentAuthGateReady(providers: AgentAuthProvider[]): boolean {
  return providers.some(
    (entry) => entry.authentication.satisfiesAccessGate && entry.status.authenticated,
  );
}

export function agentAuthRevision(providers: AgentAuthProvider[]): string {
  return providers
    .filter((entry) => entry.authentication.mode !== "external")
    .map((entry) => {
      const completion = entry.status.login.completed
        ? String(entry.status.login.startedAt || "completed")
        : "";
      return `${entry.provider}:${entry.status.authenticated ? "1" : "0"}:${completion}`;
    })
    .join("|");
}

export function unavailableManagedAgents(
  providers: AgentAuthProvider[],
  checked: boolean,
): Partial<Record<string, string>> {
  if (!checked) return {};
  const unavailable: Partial<Record<string, string>> = {};
  for (const entry of providers) {
    if (
      (entry.authentication.mode === "managed-code"
        || entry.authentication.mode === "managed-device")
      && !entry.status.authenticated
    ) {
      unavailable[entry.provider] = `Log in to ${entry.label} in Settings before selecting it.`;
    }
  }
  return unavailable;
}
