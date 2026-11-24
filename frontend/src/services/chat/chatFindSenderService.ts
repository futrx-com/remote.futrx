// Who produced a find-in-chat match, for the result list's sender line.

export type ChatFindSenderRole = "user" | "agent" | "tool" | "error";

export interface ChatFindSender {
  role: ChatFindSenderRole;
  label: string;
}

class ChatFindSenderService {
  /** Describe a match from its event type. */
  describe(kind: string): ChatFindSender {
    switch (kind) {
      case "user":
        return { role: "user", label: "You" };
      case "thinking":
        return { role: "agent", label: "Assistant · thinking" };
      case "tool_use_start":
        return { role: "tool", label: "Tool call" };
      case "tool_use_end":
        return { role: "tool", label: "Tool output" };
      case "error":
        return { role: "error", label: "Error" };
      default:
        return { role: "agent", label: "Assistant" };
    }
  }
}

export const chatFindSenderService = new ChatFindSenderService();
