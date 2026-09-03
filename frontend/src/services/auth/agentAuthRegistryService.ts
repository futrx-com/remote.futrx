import type {
  AgentAuthProvider,
  AgentAuthSnapshot,
  AgentAuthStatusKind,
} from "../../models/auth.ts";

// Questions the app asks of the agent-auth provider list: how one provider
// reads, whether the access gate is open, what changed since last time, and
// which managed agents the composer must refuse.
class AgentAuthRegistryService {
  statusKind(entry: AgentAuthProvider): AgentAuthStatusKind {
    if (entry.authentication.mode === "none") return "no-auth";
    if (entry.status.authenticated) return "authenticated";
    if (entry.authentication.mode === "external") return "external";
    return "unconfigured";
  }

  updateProvider(
    providers: AgentAuthProvider[],
    provider: string,
    status: AgentAuthSnapshot,
  ): AgentAuthProvider[] {
    return providers.map((entry) =>
      entry.provider === provider ? { ...entry, status } : entry
    );
  }

  gateReady(providers: AgentAuthProvider[]): boolean {
    return providers.some(
      (entry) => entry.authentication.satisfiesAccessGate && entry.status.authenticated,
    );
  }

  /** A value that changes whenever a managed login does — what effects watch
   *  instead of the provider array, which is rebuilt on every poll. */
  revision(providers: AgentAuthProvider[]): string {
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

  /** Managed agents that cannot be picked yet, mapped to the reason shown in
   *  the composer. Empty until the auth snapshot has actually been checked,
   *  so a slow first load never blames the user for not logging in. */
  unavailableReasons(
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
}

export const agentAuthRegistryService = new AgentAuthRegistryService();
