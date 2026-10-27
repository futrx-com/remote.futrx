import type { CreateChatInput } from "../../models/chat";
import type { AgentAuthProvider } from "../../models/auth";
import type { UserSettings } from "../../models/settings";
import { resolveRememberedAccountId } from "../../services/auth/agentAccountSelectionService.ts";

/**
 * The create-chat payload seeded from the user's saved chat preferences. A
 * known provider status repairs a deleted or cross-provider remembered account
 * before the new chat is persisted.
 *
 * A loose chat omits projectId entirely rather than sending it as undefined:
 * every field on CreateChatInput is optional, so an explicit undefined and an
 * absent key serialize differently once the payload reaches JSON.
 */
export function createChatInput(
  settings: Pick<UserSettings, "chat" | "projectChat">,
  projectId?: string,
  authProviders: readonly AgentAuthProvider[] = [],
): CreateChatInput {
  const chat = projectId ? settings.projectChat : settings.chat;
  return {
    provider: chat.provider,
    accountId: resolveRememberedAccountId(authProviders, chat.provider, chat.accountId),
    model: chat.model,
    mode: chat.mode,
    reasoningEffort: chat.reasoningEffort,
    serviceTier: chat.serviceTier,
    approvalPolicy: chat.approvalPolicy,
    sandboxPolicy: chat.sandboxPolicy,
    ...(projectId ? { projectId } : {}),
  };
}
