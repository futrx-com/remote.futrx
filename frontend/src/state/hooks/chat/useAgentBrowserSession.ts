import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { agentBrowserApi } from "../../../api/agents/agentBrowserApi";
import {
  AGENT_BROWSER_HEARTBEAT_INTERVAL_MS,
  AGENT_BROWSER_POLL_INTERVAL_MS,
} from "../../../config/agents";
import type { AgentBrowserInfo, AgentBrowserStatus } from "../../../models/project";
import { agentBrowserStatusState } from "./agentBrowserStatusState.ts";

// useAgentBrowserSession asks the backend to bring up the in-container Agent
// Browser and tracks its status over project REST endpoints. Pixels do NOT
// flow here: once ready, the noVNC view loads as an iframe from the dev-URL
// proxy. Closing the drawer stops only the human noVNC view; the agent-facing
// browser core keeps running until explicitly stopped or reaped for idleness.
export function useAgentBrowserSession({ projectId, enabled }: { projectId: string; enabled: boolean }) {
  const [status, setStatus] = useState<AgentBrowserStatus>("idle");
  const [guiUrl, setGuiUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);
  const mountedRef = useRef(true);
  // Held outside the effect so stop() can silence the heartbeat: the effect
  // does not re-run on stop, so its own cleanup would not fire until the drawer
  // closes.
  const heartbeatRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    return () => {
      mountedRef.current = false;
      requestRef.current++;
    };
  }, []);

  // All three setters run in every branch, so applying them in one fixed order
  // is the same result the cascade produced.
  const applyInfo = useCallback((info: AgentBrowserInfo): boolean => {
    const view = agentBrowserStatusState.resolve(info);
    setGuiUrl(view.guiUrl);
    setError(view.error);
    setStatus(view.status);
    return view.keepPolling;
  }, []);

  const stopHeartbeat = useCallback(() => {
    if (heartbeatRef.current === undefined) return;
    window.clearInterval(heartbeatRef.current);
    heartbeatRef.current = undefined;
  }, []);

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!enabled || !projectId) {
      setStatus("idle");
      setGuiUrl("");
      setError(null);
      return;
    }

    let disposed = false;
    let pollTimer: number | undefined;
    setStatus("starting");
    setGuiUrl("");
    setError(null);

    const isCurrent = () => mountedRef.current && !disposed && requestRef.current === requestId;

    async function pollStatus() {
      try {
        const info = await agentBrowserApi.fetchAgentBrowserStatus(projectId);
        if (!isCurrent()) return;
        if (applyInfo(info)) {
          pollTimer = window.setTimeout(pollStatus, AGENT_BROWSER_POLL_INTERVAL_MS);
        }
      } catch (err) {
        if (!isCurrent()) return;
        setError((err as Error).message || "Failed to check the agent browser.");
        setStatus("error");
      }
    }

    async function heartbeatStatus() {
      try {
        const info = await agentBrowserApi.fetchAgentBrowserStatus(projectId);
        if (isCurrent()) applyInfo(info);
      } catch {
        // The fast start poll surfaces startup errors. Heartbeats should keep
        // activity fresh without making a transient status miss tear down UI.
      }
    }

    agentBrowserApi.startAgentBrowser(projectId)
      .then((info) => {
        if (!isCurrent()) return;
        if (applyInfo(info)) {
          pollTimer = window.setTimeout(pollStatus, AGENT_BROWSER_POLL_INTERVAL_MS);
        }
      })
      .catch((err) => {
        if (!isCurrent()) return;
        setError((err as Error).message || "Failed to start the agent browser.");
        setStatus("error");
      });
    heartbeatRef.current = window.setInterval(() => {
      void heartbeatStatus();
    }, AGENT_BROWSER_HEARTBEAT_INTERVAL_MS);

    return () => {
      disposed = true;
      requestRef.current++;
      if (pollTimer !== undefined) window.clearTimeout(pollTimer);
      stopHeartbeat();
      void agentBrowserApi.stopAgentBrowser(projectId, "view").catch(() => {});
    };
  }, [projectId, enabled, applyInfo, stopHeartbeat]);

  const stop = useCallback(() => {
    if (!projectId) return;
    stopHeartbeat();
    const requestId = ++requestRef.current;
    setStatus("stopped");
    setGuiUrl("");
    setError(null);
    agentBrowserApi.stopAgentBrowser(projectId)
      .then(() => {
        if (!mountedRef.current || requestRef.current !== requestId) return;
        setGuiUrl("");
        setStatus("stopped");
      })
      .catch((err) => {
        if (!mountedRef.current || requestRef.current !== requestId) return;
        setError((err as Error).message || "Failed to stop the agent browser.");
        setStatus("error");
      });
  }, [projectId, stopHeartbeat]);

  return { status, guiUrl, error, stop };
}
