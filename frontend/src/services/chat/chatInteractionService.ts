import type {
  ApprovalReviewAction,
  ChatInteractionIntent,
  ChatInteractionWireResponse,
} from "../../models/chatInteraction";

class ChatInteractionService {
  private readonly legacyApprovalMethods = new Set([
    "execCommandApproval",
    "applyPatchApproval",
  ]);

  isTerminalAgentStatus(status: string): boolean {
    return ["completed", "failed", "interrupted", "cancelled", "canceled", "turnEnded"].includes(status);
  }

  delegatedTaskTarget(data: { stopInteractionId?: unknown }, status: string) {
    const id = data.stopInteractionId;
    if (typeof id !== "string" || !id || this.isTerminalAgentStatus(status)) return undefined;
    return { id, method: "kimi/task" };
  }

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
      case "dismiss_questions":
        return { result: { dismiss: true } };
      case "review_approval":
        return { result: this.reviewResponse(intent.action, intent.feedback, intent.optionLabel) };
      case "control_agent":
        return { result: { action: intent.action === "stop" ? "cancel" : "detach" } };
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

  private reviewResponse(action: ApprovalReviewAction, feedback: string, optionLabel?: string) {
    return {
      decision: action === "allow_once" ? "accept" : action === "allow_session" ? "acceptForSession" : "decline",
      feedback,
      selected_label: action === "revise_plan" ? "Revise" : action === "reject_plan" ? "Reject and Exit" : optionLabel,
    };
  }

  private approvalDecision(method: string, scope: "once" | "session"): string {
    if (this.legacyApprovalMethods.has(method)) {
      return scope === "session" ? "approved_for_session" : "approved";
    }
    return scope === "session" ? "acceptForSession" : "accept";
  }
}

export const chatInteractionService = new ChatInteractionService();
