import type {
  ChatInteractionIntent,
  ChatInteractionWireResponse,
} from "../../models/chatInteraction";

class ChatInteractionService {
  private readonly legacyApprovalMethods = new Set([
    "execCommandApproval",
    "applyPatchApproval",
  ]);

  supportsApprovalCancellation(method: string): boolean {
    return !this.legacyApprovalMethods.has(method);
  }

  encodeResponse(
    method: string,
    intent: ChatInteractionIntent
  ): ChatInteractionWireResponse {
    switch (intent.kind) {
      case "answer_questions":
        return {
          result: {
            answers: Object.fromEntries(
              Object.entries(intent.answers).map(([id, answers]) => [id, { answers }])
            ),
          },
        };
      case "approve":
        return {
          result: {
            decision: this.approvalDecision(method, intent.scope),
          },
        };
      case "deny_approval":
        return {
          result: {
            decision: this.legacyApprovalMethods.has(method)
              ? { denied: { rejection: "Denied by user" } }
              : "decline",
          },
        };
      case "cancel_approval":
        return { result: { decision: "cancel" } };
      case "grant_permissions":
        return {
          result: {
            permissions: intent.permissions,
            scope: intent.scope,
          },
        };
      case "deny_permissions":
        return { result: { permissions: {}, scope: "turn" } };
      case "accept_elicitation":
        return { result: { action: "accept", content: intent.content } };
      case "decline_elicitation":
        return { result: { action: "decline" } };
      case "cancel_elicitation":
        return { result: { action: "cancel" } };
      case "submit_provider_result":
        return { result: intent.result };
      case "decline_unsupported":
        return {
          error: {
            code: -32601,
            message: "Unsupported provider request declined by user",
          },
        };
    }
  }

  private approvalDecision(method: string, scope: "once" | "session"): string {
    if (this.legacyApprovalMethods.has(method)) {
      return scope === "session" ? "approved_for_session" : "approved";
    }
    return scope === "session" ? "acceptForSession" : "accept";
  }
}

export const chatInteractionService = new ChatInteractionService();
