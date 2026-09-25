import type { AgentCapabilityCatalogSnapshot } from "../models/agentCapabilities";

/** How often the drawer asks the backend how the agent browser is coming up. */
export const AGENT_BROWSER_POLL_INTERVAL_MS = 1_500;
/** How often an open drawer tells the backend the browser is still watched. */
export const AGENT_BROWSER_HEARTBEAT_INTERVAL_MS = 15_000;

export const EMPTY_AGENT_CAPABILITY_CATALOG_SNAPSHOT: AgentCapabilityCatalogSnapshot = {
  catalog: null,
  loading: false,
  refreshing: false,
  error: "",
};
