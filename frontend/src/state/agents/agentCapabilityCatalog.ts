import { capabilitiesApi } from "../../api/agents/capabilitiesApi";
import { AgentCapabilityCatalogStore } from "./agentCapabilityCatalogStore";

export {
  type AgentCapabilityCatalogSnapshot,
} from "./agentCapabilityCatalogStore";

export const agentCapabilityCatalogStore = new AgentCapabilityCatalogStore(
  capabilitiesApi.list,
);
