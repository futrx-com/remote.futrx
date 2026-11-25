import { useEffect, useState } from "preact/hooks";
import {
  searchWorkspace,
  type WorkspaceSearchOutcome,
  type WorkspaceSearchRequest,
} from "../../../api/chat/workspaceSearchApi.ts";
import { WORKSPACE_SEARCH_DEBOUNCE_MS } from "../../../config/search.ts";
import type { ChatMeta } from "../../../models/chat.ts";
import type { ProjectMeta } from "../../../models/project.ts";

const emptyOutcome: WorkspaceSearchOutcome = {
  facetValues: { project: [], provider: [], model: [], mode: [], status: [], effort: [], tier: [], skill: [] },
  hits: [],
  counts: {
    project: new Map(), provider: new Map(), model: new Map(), mode: new Map(),
    status: new Map(), effort: new Map(), tier: new Map(), skill: new Map(),
  },
  counted: false,
  total: 0,
};

/** Own the debounce and ignore responses superseded by changes to the request. */
export function useWorkspaceSearchResults(
  request: WorkspaceSearchRequest,
  enabled: boolean,
  chats: readonly ChatMeta[],
  projects: readonly ProjectMeta[],
) {
  // Refresh counts without invalidating results for the same query and filters.
  const { withCounts: _withCounts, ...criteria } = request;
  const key = JSON.stringify(criteria);
  const [result, setResult] = useState<{
    key: string; outcome: WorkspaceSearchOutcome; error: string | null;
  } | null>(null);
  const current = result?.key === key ? result : null;

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      searchWorkspace(request).then((outcome) => {
        if (!cancelled) setResult({ key, outcome, error: null });
      }).catch((error: Error) => {
        if (!cancelled) setResult({ key, outcome: emptyOutcome, error: error.message });
      });
    }, WORKSPACE_SEARCH_DEBOUNCE_MS);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [enabled, key, request, chats, projects]);

  return {
    outcome: current?.outcome ?? emptyOutcome,
    loading: enabled && !current,
    error: current?.error ?? null,
  };
}
