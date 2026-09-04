export type ChatInteractionIntent =
  | { kind: "answer_questions"; answers: Record<string, string[]> }
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
  options?: Array<{ label?: string; description?: string }>;
  isOther?: boolean;
  isSecret?: boolean;
}
