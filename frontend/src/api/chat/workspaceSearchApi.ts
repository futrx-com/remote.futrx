import { requestJson } from "../apiRequest.ts";
import { API_ROUTES } from "../../config/routes.ts";
import { FACET_IDS } from "../../config/search.ts";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type {
  DateField,
  FacetSelections,
  SearchHit,
  SearchOutcome,
  SortId,
} from "../../models/search.ts";

export interface WorkspaceSearchSelection {
  facets: FacetSelections;
  dateField: DateField;
  from: number | null;
  to: number | null;
}

interface WorkspaceSearchHitPayload extends Omit<SearchHit, "doc"> {
  chat: ChatMeta;
  project: ProjectMeta | null;
}

interface WorkspaceSearchPayload {
  hits: WorkspaceSearchHitPayload[];
  counts: Record<(typeof FACET_IDS)[number], Record<string, number>>;
  facetValues: FacetSelections;
  counted: boolean;
  total: number;
}

export type WorkspaceSearchOutcome = SearchOutcome & { facetValues: FacetSelections };

export interface WorkspaceSearchRequest extends WorkspaceSearchSelection {
  query: string;
  sort: SortId;
  withCounts: boolean;
}

export async function searchWorkspace(body: WorkspaceSearchRequest): Promise<WorkspaceSearchOutcome> {
  const result = await requestJson<WorkspaceSearchPayload>("POST", API_ROUTES.chats.searchMetadata, body);
  const counts: SearchOutcome["counts"] = {
    project: new Map(), provider: new Map(), model: new Map(), mode: new Map(),
    status: new Map(), effort: new Map(), tier: new Map(), skill: new Map(),
  };
  for (const id of FACET_IDS) counts[id] = new Map(Object.entries(result.counts[id]));
  return {
    counted: result.counted,
    facetValues: result.facetValues,
    total: result.total,
    counts,
    hits: result.hits.map(({ chat, project, ...match }) => ({
      ...match,
      doc: {
        chat,
        project,
        folded: [],
        unread: (chat.lastMessageAt || 0) > (chat.lastReadAt || 0),
      },
    })),
  };
}
