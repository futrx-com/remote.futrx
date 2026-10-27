import { createStore } from "zustand/vanilla";
import type {
  AgentCapabilitiesCatalog,
  AgentCapabilityCatalogLoadOptions,
  AgentCapabilityCatalogRequester,
  AgentCapabilityCatalogSnapshot,
  AgentCapabilityCatalogStoreActions,
  AgentCapabilityCatalogStoreState,
  ObservedAgentCapabilityScope,
} from "../../../models/agentCapabilities";
import { capabilitiesApi } from "../../../api/agents/capabilitiesApi.ts";
import { EMPTY_AGENT_CAPABILITY_CATALOG_SNAPSHOT } from "../../../config/agents.ts";

// This store keeps the last response for each normalized user and host/project
// scope only for the lifetime of the open application. The process-local
// backend cache owns freshness across browsers and devices. Retaining the last
// frontend response avoids a visual reset while a backend lookup or refresh is
// in flight; in-flight requests for the same frontend scope are coalesced.
export function createAgentCapabilityCatalogStore(request: AgentCapabilityCatalogRequester) {
  const inFlight = new Map<string, Promise<AgentCapabilitiesCatalog>>();
  const observed = new Map<string, ObservedAgentCapabilityScope>();

  return createStore<
    AgentCapabilityCatalogStoreState & AgentCapabilityCatalogStoreActions
  >()(
    (set, get) => {
      function setScope(key: string, snapshot: AgentCapabilityCatalogSnapshot): void {
        set((state) => {
          const scopes = new Map(state.scopes);
          scopes.set(key, snapshot);
          return { scopes };
        });
      }

      function load(
        userId: string,
        projectId?: string,
        options: AgentCapabilityCatalogLoadOptions = {},
      ): Promise<AgentCapabilitiesCatalog> {
        const key = catalogKey(userId, projectId);
        const existing = inFlight.get(key);
        if (existing) return existing;

        const running = (async () => {
          try {
            const catalog = await request(projectId, { refresh: !!options.force });
            inFlight.delete(key);
            setScope(key, {
              catalog,
              loading: false,
              refreshing: false,
              error: "",
            });
            return catalog;
          } catch (cause) {
            inFlight.delete(key);
            const current = scopeSnapshot(get(), key);
            setScope(key, {
              ...current,
              loading: false,
              refreshing: false,
              error: errorMessage(cause),
            });
            throw cause;
          }
        })();

        inFlight.set(key, running);
        const current = scopeSnapshot(get(), key);
        setScope(key, {
          ...current,
          loading: !current.catalog,
          refreshing: true,
          error: "",
        });
        return running;
      }

      return {
        scopes: new Map(),
        observe: (userId, projectId) => {
          const key = catalogKey(userId, projectId);
          const scope = observed.get(key) ?? {
            userId: normalizeUserId(userId),
            projectId: projectId || "",
            observers: 0,
          };
          scope.observers += 1;
          observed.set(key, scope);
          let isObserved = true;
          return () => {
            if (!isObserved) return;
            isObserved = false;
            scope.observers -= 1;
            if (scope.observers === 0) observed.delete(key);
          };
        },
        load,
        invalidateUser: (userId) => {
          // A managed host-auth change can alter every catalog. Request a
          // force-refresh for scopes currently observed by this browser; an
          // existing request for the same scope remains coalesced.
          const normalizedUser = normalizeUserId(userId);
          for (const scope of observed.values()) {
            if (scope.userId !== normalizedUser) continue;
            void load(scope.userId, scope.projectId || undefined, { force: true })
              .catch(() => undefined);
          }
        },
        removeProject: (userId, projectId) => {
          const key = catalogKey(userId, projectId);
          const refreshing = inFlight.has(key);
          if (refreshing) {
            setScope(key, {
              catalog: null,
              loading: true,
              refreshing: true,
              error: "",
            });
            return;
          }
          set((state) => {
            const scopes = new Map(state.scopes);
            scopes.delete(key);
            return { scopes };
          });
        },
      };
    },
  );
}

export function selectAgentCapabilityCatalog(userId: string, projectId?: string) {
  const key = catalogKey(userId, projectId);
  return (state: AgentCapabilityCatalogStoreState): AgentCapabilityCatalogSnapshot =>
    scopeSnapshot(state, key);
}

function scopeSnapshot(
  state: AgentCapabilityCatalogStoreState,
  key: string,
): AgentCapabilityCatalogSnapshot {
  return state.scopes.get(key) ?? EMPTY_AGENT_CAPABILITY_CATALOG_SNAPSHOT;
}

function catalogKey(userId: string, projectId?: string): string {
  return JSON.stringify([normalizeUserId(userId), projectId || ""]);
}

function normalizeUserId(userId: string): string {
  return userId.trim().toLowerCase() || "anonymous";
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error && cause.message
    ? cause.message
    : "Could not load agent capabilities";
}

// Every caller observes one instance so the retained responses above are shared
// across the application rather than rebuilt per consumer.
export const agentCapabilityCatalogStore = createAgentCapabilityCatalogStore(
  capabilitiesApi.list,
);
