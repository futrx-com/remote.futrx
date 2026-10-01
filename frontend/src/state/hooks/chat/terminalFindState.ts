import type { FindStatus } from "./useChatFind.ts";

/** What the terminal's search addon last reported. */
export interface TerminalSearchResults {
  /** The active match, or -1 once there are more matches than the addon highlights. */
  index: number;
  count: number;
}

class TerminalFindState {
  /** The find bar's status for a query and the addon's latest results. */
  status(query: string, results: TerminalSearchResults | null): FindStatus {
    if (!query) return { kind: "idle" };
    if (!results || results.count <= 0) return { kind: "empty" };
    // Past the highlight limit the addon still steps through matches but stops
    // saying which one is active; count from the top rather than show "0 of n".
    return {
      kind: "matched",
      position: results.index >= 0 ? results.index + 1 : 1,
      total: results.count,
    };
  }
}

export const terminalFindState = new TerminalFindState();
