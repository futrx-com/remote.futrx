import type {
  AgentAuthAccountsSnapshot,
  AgentAuthProvider,
} from "../../models/auth";
import type { ChatProvider } from "../../models/chat";

export function accountsForProvider(
  authProviders: readonly AgentAuthProvider[],
  provider: ChatProvider,
): AgentAuthAccountsSnapshot | undefined {
  return authProviders.find((entry) => entry.provider === provider)?.status.accounts;
}

export function resolveProviderAccountId(
  accounts: AgentAuthAccountsSnapshot | undefined,
  requestedAccountId: string,
): string {
  if (requestedAccountId && accounts?.items.some((account) => account.id === requestedAccountId)) {
    return requestedAccountId;
  }
  if (accounts?.activeAccountId && accounts.items.some(
    (account) => account.id === accounts.activeAccountId,
  )) {
    return accounts.activeAccountId;
  }
  return accounts?.items[0]?.id || "";
}

// Preserve a remembered account while provider status is still loading. Once
// the provider is known, reject an account from another provider (or one that
// was deleted) and use that provider's current default instead.
export function resolveRememberedAccountId(
  authProviders: readonly AgentAuthProvider[],
  provider: ChatProvider,
  rememberedAccountId: string,
): string {
  const authProvider = authProviders.find((entry) => entry.provider === provider);
  if (!authProvider) return rememberedAccountId.trim();
  return resolveProviderAccountId(authProvider.status.accounts, rememberedAccountId);
}
