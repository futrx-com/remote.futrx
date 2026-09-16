import type { FlowMapState } from "../../models/flow";

export const flowService = {
  async getFlowState(chatId: string): Promise<FlowMapState> {
    const res = await fetch(`/api/chats/${encodeURIComponent(chatId)}/flow`, {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      throw new Error(`Failed to fetch flow state: ${res.statusText}`);
    }
    return res.json();
  },
};
