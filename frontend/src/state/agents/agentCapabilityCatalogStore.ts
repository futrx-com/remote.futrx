import type { AgentCapabilitiesCatalog } from "../../models/agentCapabilities";

type CatalogRequester = (
  projectId?: string,
  options?: { refresh?: boolean },
) => Promise<AgentCapabilitiesCatalog>;

export interface AgentCapabilityCatalogSnapshot {
  catalog: AgentCapabilitiesCatalog | null;
  loading: boolean;
  refreshing: boolean;
  error: string;
}

// This store keeps the last response for each normalized user and host/project
// scope only for the lifetime of the open application. The process-local
// backend cache owns freshness across browsers and devices. Retaining the last
// frontend response avoids a visual reset while a backend lookup or refresh is
// in flight; in-flight requests for the same frontend scope are coalesced.
export class AgentCapabilityCatalogStore {
  private readonly catalogs = new Map<string, AgentCapabilitiesCatalog>();
  private readonly errors = new Map<string, string>();
  private readonly inFlight = new Map<string, Promise<AgentCapabilitiesCatalog>>();
  private readonly listeners = new Map<string, Set<() => void>>();
  private readonly request: CatalogRequester;

  constructor(request: CatalogRequester) {
    this.request = request;
  }

  read(userId: string, projectId?: string): AgentCapabilityCatalogSnapshot {
    const key = catalogKey(userId, projectId);
    const catalog = this.catalogs.get(key) ?? null;
    const refreshing = this.inFlight.has(key);
    return {
      catalog,
      loading: refreshing && !catalog,
      refreshing,
      error: this.errors.get(key) ?? "",
    };
  }

  subscribe(userId: string, projectId: string | undefined, listener: () => void): () => void {
    const key = catalogKey(userId, projectId);
    const listeners = this.listeners.get(key) ?? new Set<() => void>();
    listeners.add(listener);
    this.listeners.set(key, listeners);
    return () => {
      listeners.delete(listener);
      if (listeners.size === 0) this.listeners.delete(key);
    };
  }

  load(
    userId: string,
    projectId?: string,
    options: { force?: boolean } = {},
  ): Promise<AgentCapabilitiesCatalog> {
    const key = catalogKey(userId, projectId);
    const existing = this.inFlight.get(key);
    if (existing) return existing;

    this.errors.delete(key);
    const running = (async () => {
      try {
        const catalog = await this.request(projectId, { refresh: !!options.force });
        this.catalogs.set(key, catalog);
        this.errors.delete(key);
        return catalog;
      } catch (cause) {
        this.errors.set(key, errorMessage(cause));
        throw cause;
      } finally {
        this.inFlight.delete(key);
        this.notify(key);
      }
    })();

    this.inFlight.set(key, running);
    this.notify(key);
    return running;
  }

  invalidateUser(userId: string): void {
    // A managed host-auth change can alter every catalog. Request a
    // force-refresh for scopes currently observed by this browser; an existing
    // request for the same scope remains coalesced.
    const normalizedUser = normalizeUserId(userId);
    for (const key of this.listeners.keys()) {
      const scope = parseCatalogKey(key);
      if (!scope || scope[0] !== normalizedUser) continue;
      void this.load(normalizedUser, scope[1] || undefined, { force: true })
        .catch(() => undefined);
    }
  }

  invalidateProject(userId: string, projectId?: string): void {
    // Starting a project may change its CLI/configuration. Request a fresh
    // frontend snapshot and backend entry; reuse any request already in flight.
    void this.load(userId, projectId, { force: true }).catch(() => undefined);
  }

  removeProject(userId: string, projectId: string): void {
    const key = catalogKey(userId, projectId);
    this.catalogs.delete(key);
    this.errors.delete(key);
    this.notify(key);
  }

  private notify(key: string): void {
    for (const listener of this.listeners.get(key) ?? []) listener();
  }
}

function catalogKey(userId: string, projectId?: string): string {
  return JSON.stringify([normalizeUserId(userId), projectId || ""]);
}

function parseCatalogKey(key: string): [string, string] | null {
  try {
    const value = JSON.parse(key) as unknown;
    return Array.isArray(value)
      && value.length === 2
      && typeof value[0] === "string"
      && typeof value[1] === "string"
      ? [value[0], value[1]]
      : null;
  } catch {
    return null;
  }
}

function normalizeUserId(userId: string): string {
  return userId.trim().toLowerCase() || "anonymous";
}

function errorMessage(cause: unknown): string {
  return cause instanceof Error && cause.message
    ? cause.message
    : "Could not load agent capabilities";
}
