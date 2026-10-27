import type { ChatEvent } from "../../../models/chat";
import {
  EMPTY_CHAT_USAGE_TOTALS,
  type ChatUsagePayload,
  type ChatUsageTotals,
} from "../../../models/chatUsage.ts";

// Token totals for a chat, summed from the usage each `complete` event carries.
// Kept apart from block assembly so a change to the token model — a new cache
// field, a new provider spelling — does not touch message rendering.
class ChatUsageAccumulator {
  // Totals across every event that reports usage. Returns
  // EMPTY_CHAT_USAGE_TOTALS itself when nothing does, so an empty transcript
  // keeps a stable reference.
  totalFor(events: ChatEvent[]): ChatUsageTotals {
    let totals = EMPTY_CHAT_USAGE_TOTALS;
    for (const event of events) {
      totals = this.add(totals, event);
    }
    return totals;
  }

  private add(totals: ChatUsageTotals, event: ChatEvent): ChatUsageTotals {
    if (event.type !== "complete" || !event.usage) return totals;

    try {
      const usage = (typeof event.usage === "string" ? JSON.parse(event.usage) : event.usage) as ChatUsagePayload | null;
      if (!usage) return totals;
      return {
        inputTokens: totals.inputTokens + (usage.input_tokens ?? 0),
        outputTokens: totals.outputTokens + (usage.output_tokens ?? 0),
        cacheReadTokens: totals.cacheReadTokens + (usage.cache_read_input_tokens ?? 0),
        cacheWriteTokens: totals.cacheWriteTokens + (usage.cache_creation_input_tokens ?? 0),
      };
    } catch {
      return totals;
    }
  }
}

export const chatUsageAccumulator = new ChatUsageAccumulator();
