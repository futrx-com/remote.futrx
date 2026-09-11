import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { applicationsApi } from "../../../api/applicationsApi";
import { projectApi } from "../../../api/projectApi";
import type {
  AppCredentials,
  AppImage,
  AppInstallRequest,
  AppInstance,
  AppScope,
} from "../../../models/application";
import type { ProjectMeta } from "../../../models/project";

/** Everything the Applications UI needs, independent of scope. */
export interface ApplicationsController {
  scope: AppScope;
  /** Set for project scope; the project these instances belong to. */
  projectId?: string;
  catalog: AppImage[];
  catalogLoading: boolean;
  instances: AppInstance[];
  loading: boolean;
  error?: string;
  reload: () => Promise<void>;
  install: (req: AppInstallRequest) => Promise<void>;
  start: (appId: string) => Promise<void>;
  stop: (appId: string) => Promise<void>;
  setPort: (appId: string, port: number) => Promise<void>;
  uninstall: (appId: string) => Promise<void>;
  credentials: (appId: string) => Promise<AppCredentials>;
}

// backend bindings differ only by scope; the UI logic below is shared.
interface Bindings {
  list: () => Promise<AppInstance[]>;
  install: (req: AppInstallRequest) => Promise<AppInstance>;
  start: (appId: string) => Promise<AppInstance>;
  stop: (appId: string) => Promise<AppInstance>;
  setPort: (appId: string, port: number) => Promise<AppInstance>;
  uninstall: (appId: string) => Promise<unknown>;
  credentials: (appId: string) => Promise<AppCredentials>;
}

/**
 * Notified once this controller has established what is installed — after a
 * load as well as after an install or uninstall. The consumer that matters is
 * the extension host: it holds derived state, the `ui/` modules it has loaded,
 * and a change made anywhere else reaches it through no other signal.
 */
type ApplicationsSettled = () => void;

interface CoreOptions {
  scope: AppScope;
  enabled: boolean;
  bindings: Bindings | null;
  onApplicationsSettled?: ApplicationsSettled;
  projectId?: string;
}

function useApplicationsCore({
  scope,
  enabled,
  bindings,
  onApplicationsSettled,
  projectId,
}: CoreOptions): ApplicationsController {
  const [catalog, setCatalog] = useState<AppImage[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [instances, setInstances] = useState<AppInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | undefined>();
  // Held in a ref so every operation below keeps one stable identity: the load
  // effect calls this too, and a caller passing a fresh closure per render
  // would otherwise turn that effect into a loop.
  const settledRef = useRef(onApplicationsSettled);
  settledRef.current = onApplicationsSettled;
  const notifySettled = useCallback(() => settledRef.current?.(), []);

  const reload = useCallback(async () => {
    if (!enabled || !bindings) return;
    setLoading(true);
    setError(undefined);
    try {
      const data = await bindings.list();
      setInstances(data ?? []);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [enabled, bindings]);

  const loadCatalog = useCallback(async () => {
    if (!enabled) return;
    setCatalogLoading(true);
    try {
      const data = await applicationsApi.catalog();
      setCatalog(data ?? []);
    } catch {
      // Catalog failures surface via the empty grid; instance errors are shown.
    } finally {
      setCatalogLoading(false);
    }
  }, [enabled]);

  useEffect(() => {
    let cancelled = false;
    if (!enabled) return;
    void (async () => {
      await Promise.all([loadCatalog(), reload()]);
      if (cancelled) return;
      // Opening a surface reconciles the extension host, not just changing
      // something on it. A change made anywhere else — another tab, another
      // administrator, a server that restarted without the image — reaches
      // this tab through no other path, and without this the surface can list
      // no installed apps while still rendering an uninstalled one's panel.
      notifySettled();
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, loadCatalog, reload, notifySettled]);

  const upsert = useCallback((inst: AppInstance) => {
    setInstances((current) => {
      const next = [...current];
      const i = next.findIndex((x) => x.id === inst.id);
      if (i >= 0) next[i] = inst;
      else next.push(inst);
      return next;
    });
  }, []);

  const install = useCallback(
    async (req: AppInstallRequest) => {
      if (!bindings) return;
      const inst = await bindings.install(req);
      upsert(inst);
      notifySettled();
    },
    [bindings, upsert, notifySettled],
  );

  const start = useCallback(
    async (appId: string) => {
      if (!bindings) return;
      upsert(await bindings.start(appId));
      notifySettled();
    },
    [bindings, upsert, notifySettled],
  );

  const stop = useCallback(
    async (appId: string) => {
      if (!bindings) return;
      upsert(await bindings.stop(appId));
      notifySettled();
    },
    [bindings, upsert, notifySettled],
  );

  const setPort = useCallback(
    async (appId: string, port: number) => {
      if (!bindings) return;
      upsert(await bindings.setPort(appId, port));
    },
    [bindings, upsert]
  );

  const uninstall = useCallback(
    async (appId: string) => {
      if (!bindings) return;
      await bindings.uninstall(appId);
      setInstances((current) => current.filter((x) => x.id !== appId));
      notifySettled();
    },
    [bindings, notifySettled],
  );

  const credentials = useCallback(
    (appId: string) => {
      if (!bindings) return Promise.reject(new Error("applications unavailable"));
      return bindings.credentials(appId);
    },
    [bindings]
  );

  return {
    scope,
    projectId,
    catalog,
    catalogLoading,
    instances,
    loading,
    error,
    reload,
    install,
    start,
    stop,
    setPort,
    uninstall,
    credentials,
  };
}

/** Global (server-wide) applications; admin-only. */
export function useGlobalApplications(
  enabled: boolean,
  onApplicationsSettled?: ApplicationsSettled,
): ApplicationsController {
  const bindings = useMemo<Bindings>(
    () => ({
      list: applicationsApi.listGlobal,
      install: applicationsApi.install,
      start: applicationsApi.start,
      stop: applicationsApi.stop,
      setPort: applicationsApi.setPort,
      uninstall: applicationsApi.uninstall,
      credentials: applicationsApi.credentials,
    }),
    []
  );
  return useApplicationsCore({
    scope: "global",
    enabled,
    bindings,
    onApplicationsSettled,
  });
}

/** Applications scoped to a single project. */
export function useProjectApplications(
  project: ProjectMeta | null,
  enabled: boolean,
  onApplicationsSettled?: ApplicationsSettled,
): ApplicationsController {
  const id = project?.id ?? null;
  const bindings = useMemo<Bindings | null>(
    () =>
      id
        ? {
            list: () => projectApi.listApplications(id),
            install: (req) => projectApi.installApplication(id, req),
            start: (appId) => projectApi.startApplication(id, appId),
            stop: (appId) => projectApi.stopApplication(id, appId),
            setPort: (appId, port) => projectApi.setApplicationPort(id, appId, port),
            uninstall: (appId) => projectApi.uninstallApplication(id, appId),
            credentials: (appId) => projectApi.applicationCredentials(id, appId),
          }
        : null,
    [id]
  );
  return useApplicationsCore({
    scope: "project",
    enabled: enabled && !!id,
    bindings,
    onApplicationsSettled,
    projectId: id ?? undefined,
  });
}
