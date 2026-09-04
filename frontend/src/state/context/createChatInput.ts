import type { CreateChatInput } from "../../models/chat";
import type { UserSettings } from "../../models/settings";

/**
 * The create-chat payload seeded from the user's saved chat preferences.
 *
 * A loose chat omits projectId entirely rather than sending it as undefined:
 * every field on CreateChatInput is optional, so an explicit undefined and an
 * absent key serialize differently once the payload reaches JSON.
 */
export function createChatInput(
  settings: Pick<UserSettings, "chat" | "projectChat">,
  projectId?: string
): CreateChatInput {
  const chat = projectId ? settings.projectChat : settings.chat;
  return {
    provider: chat.provider,
    model: chat.model,
    mode: chat.mode,
    reasoningEffort: chat.reasoningEffort,
    serviceTier: chat.serviceTier,
    approvalPolicy: chat.approvalPolicy,
    sandboxPolicy: chat.sandboxPolicy,
    ...(projectId ? { projectId } : {}),
  };
}
