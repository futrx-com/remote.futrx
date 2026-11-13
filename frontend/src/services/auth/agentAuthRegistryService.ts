import type {
  AgentAuthProvider,
  AgentAuthSnapshot,
  AgentAuthStatusKind,
} from "../../models/auth.ts";

// Questions the app asks of the agent-auth provider list: how one provider
// reads, whether the access gate is open, what changed since last time, and
// which agents the composer must refuse.
class AgentAuthRegistryService {
  supportsSavedAccounts(entry: AgentAuthProvider): boolean {
    return !!entry.status.accounts
      && !entry.status.accounts.defaultOnly
      && entry.authentication.mode !== "external"
      && entry.authentication.mode !== "none";
  }

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
    return providers.some((entry) => {
      const accounts = entry.status.accounts;
      if (accounts?.defaultAllowed !== undefined) {
        // A member's filtered catalog answers whether they can use an agent.
        // Deployment onboarding readiness is enforced separately by the server.
        const hasAccountAccess = accounts.defaultAllowed || accounts.items.length > 0;
        return hasAccountAccess
          && (entry.authentication.mode === "external" || entry.status.authenticated);
      }
      return entry.authentication.satisfiesAccessGate && entry.status.authenticated;
    });
  }

  /** A value that changes whenever a login or account grant does — what effects watch
   *  instead of the provider array, which is rebuilt on every poll. */
  revision(providers: AgentAuthProvider[]): string {
    return providers
      .filter((entry) => entry.authentication.mode !== "external"
        || entry.status.accounts?.defaultAllowed !== undefined)
      .map((entry) => {
        const completion = entry.status.login.completed
          ? String(entry.status.login.startedAt || "completed")
          : "";
        const activeAccount = entry.status.accounts?.activeAccountId || "";
        const accounts = entry.status.accounts;
        const access = accounts?.defaultAllowed === undefined
          ? ""
          : JSON.stringify([accounts.defaultAllowed, accounts.items.map((account) => account.id)]);
        return `${entry.provider}:${entry.status.authenticated ? "1" : "0"}:${activeAccount}:${completion}:${access}`;
      })
      .join("|");
  }

  /** Agents that cannot be picked yet, mapped to the reason shown in
   *  the composer. Empty until the auth snapshot has actually been checked,
   *  so a slow first load never blames the user for not logging in. */
  unavailableReasons(
    providers: AgentAuthProvider[],
    checked: boolean,
  ): Partial<Record<string, string>> {
    if (!checked) return {};
    const unavailable: Partial<Record<string, string>> = {};
    for (const entry of providers) {
      if (entry.status.accounts?.defaultAllowed === false && entry.status.accounts.items.length === 0) {
        unavailable[entry.provider] = `Ask an administrator to grant access to a ${entry.label} account.`;
        continue;
      }
      if (
        entry.authentication.mode === "managed-api-key"
        && !entry.status.authenticated
      ) {
        unavailable[entry.provider] =
          `Sign in to ${entry.label} in Settings → Agents, then refresh models.`;
        continue;
      }
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
