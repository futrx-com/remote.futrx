import type { AgentCapabilitiesCatalog } from "../../models/agentCapabilities";

export function streamingPresentationFor(
  catalog: AgentCapabilitiesCatalog | null,
  provider: string | undefined,
): "blocks" | "tokens" {
  return catalog?.providers.find((candidate) => candidate.provider === provider)
    ?.features?.streamingPresentation === "blocks" ? "blocks" : "tokens";
}
