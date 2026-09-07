export type ApprovalReviewAction = "allow_once" | "allow_session" | "deny" | "revise_plan" | "reject_plan";

export type ChatInteractionIntent =
  | { kind: "answer_questions"; answers: Record<string, string[]> }
  | { kind: "dismiss_questions" }
  | { kind: "review_approval"; action: ApprovalReviewAction; feedback: string; optionLabel?: string }
  | { kind: "control_agent"; action: "stop" | "run_in_background" }
  | { kind: "approve"; scope: "once" | "session" }
  | { kind: "deny_approval" }
  | { kind: "cancel_approval" }
  | {
      kind: "grant_permissions";
      permissions: Record<string, unknown>;
      scope: "turn" | "session";
    }
  | { kind: "deny_permissions" }
  | { kind: "accept_elicitation"; content: unknown }
  | { kind: "decline_elicitation" }
  | { kind: "cancel_elicitation" }
  | { kind: "submit_provider_result"; result: unknown }
  | { kind: "decline_unsupported" };

export type ChatInteractionWireResponse =
  | { result: unknown; error?: never }
  | { result?: never; error: { code: number; message: string } };

export interface ChatInteractionQuestion {
  id?: string;
  header?: string;
  question?: string;
  body?: string;
  multiSelect?: boolean;
  options?: Array<{ id?: string; label?: string; description?: string }>;
  isOther?: boolean;
  isSecret?: boolean;
}
