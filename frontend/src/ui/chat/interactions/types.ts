import type { AssistantMessagePart } from "../../../models/chatMessage";
import type { ChatInteractionIntent } from "../../../models/chatInteraction";

export type InteractionPart = Extract<AssistantMessagePart, { kind: "interaction" }>;

export interface InteractionFormProps {
  input: Record<string, unknown>;
  disabled: boolean;
  onSubmit: (intent: ChatInteractionIntent) => void;
}
