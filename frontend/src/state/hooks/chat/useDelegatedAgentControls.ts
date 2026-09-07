import type { ChatInteractionResponder } from "../../../types/chatApi";
import { chatInteractionService } from "../../../services/chat/chatInteractionService";

export function useDelegatedAgentControls(
  data: { stopInteractionId?: unknown; runInBackground?: unknown },
  status: string,
  onRespond?: ChatInteractionResponder,
) {
  const target = chatInteractionService.delegatedTaskTarget(data, status);
  return {
    terminal: chatInteractionService.isTerminalAgentStatus(status),
    canStop: !!target && !!onRespond,
    canRunInBackground: data.runInBackground !== true,
    stop: () => target && onRespond?.(target.id, target.method, { kind: "control_agent", action: "stop" }),
    runInBackground: () => target && onRespond?.(target.id, target.method, { kind: "control_agent", action: "run_in_background" }),
  };
}
