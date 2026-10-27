import type { AgentBrowserInfo, AgentBrowserView } from "../../../models/project";

// Reads a backend status report as the three values the drawer renders. Kept
// apart from the session hook so the rules — a ready browser without an address
// is a failure, and only a browser still coming up is worth polling again — can
// be read and tested without standing up timers or a socket.
class AgentBrowserStatusState {
  resolve(info: AgentBrowserInfo): AgentBrowserView {
    if (info.status === "ready") {
      // Ready without an address is not usable: the iframe has nothing to load,
      // so surface it as a failure rather than a ready browser that shows
      // nothing.
      if (!info.url) {
        return {
          status: "error",
          guiUrl: "",
          error: "Agent browser started but returned an incomplete address.",
          keepPolling: false,
        };
      }
      return { status: "ready", guiUrl: info.url, error: null, keepPolling: false };
    }

    if (info.status === "error") {
      return {
        status: "error",
        guiUrl: "",
        error: info.error || "Failed to start the agent browser.",
        keepPolling: false,
      };
    }

    return {
      status: info.status,
      guiUrl: "",
      error: null,
      keepPolling: info.status === "starting" || info.status === "core-ready",
    };
  }
}

export const agentBrowserStatusState = new AgentBrowserStatusState();
