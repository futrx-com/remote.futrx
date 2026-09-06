import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import { applicationsApi } from "../../../api/applicationsApi";
import { projectApi } from "../../../api/projectApi";
import type {
  AppCredentials,
  AppImage,
  AppInstallRequest,
  AppInstance,
  AppPackage,
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
  /**
   * Uploaded application packages. The catalog they extend is server-wide, so
   * they are the same list wherever it is shown; only an administrator may add
   * to or remove from it.
   */
  packages: AppPackage[];
  managesPackages: boolean;
  /** Adds a .zip to the catalog, or replaces the package with the same id. */
  uploadPackage: (file: File) => Promise<AppPackage>;
  /**
   * Removes an uploaded app. `uninstallInstalled` uninstalls every copy first;
   * without it a package that is still installed is refused, and the refusal
   * names where it is installed.
   */
  removePackage: (packageId: string, uninstallInstalled?: boolean) => Promise<void>;
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

type ApplicationsChanged = () => void;

interface CoreOptions {
  scope: AppScope;
  enabled: boolean;
  /**
   * Whether this caller may manage the uploaded-package catalog. It is an
   * administrator check, not a scope check: the catalog is server-wide and
   * uploading one adds code that runs with the server's privileges, so the
   * answer is the same in a project as it is in Settings.
   */
  managesPackages: boolean;
  bindings: Bindings | null;
  onApplicationsChanged?: ApplicationsChanged;
  projectId?: string;
}

function useApplicationsCore({
  scope,
  enabled,
  managesPackages,
  bindings,
  onApplicationsChanged,
  projectId,
}: CoreOptions): ApplicationsController {
  const [catalog, setCatalog] = useState<AppImage[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [instances, setInstances] = useState<AppInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [packages, setPackages] = useState<AppPackage[]>([]);

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

  const loadPackages = useCallback(async () => {
    if (!enabled || !managesPackages) return;
    try {
      const data = await applicationsApi.packages();
      setPackages(data ?? []);
    } catch {
      // A server built without a package store answers 503 here. That is not
      // an error to show: it simply has no uploaded applications to list.
      setPackages([]);
    }
  }, [enabled, managesPackages]);

  useEffect(() => {
    let cancelled = false;
    if (!enabled) return;
    void (async () => {
      await Promise.all([loadCatalog(), reload(), loadPackages()]);
      if (cancelled) return;
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, loadCatalog, reload, loadPackages]);

  // Uploading and removing both change what the catalog holds, so both end by
  // reloading it — the new card has to appear without a page refresh, and a
  // removed one has to stop offering an install that would now fail.
  const uploadPackage = useCallback(
    async (file: File) => {
      const uploaded = await applicationsApi.uploadPackage(file);
      await Promise.all([loadCatalog(), loadPackages()]);
      onApplicationsChanged?.();
      return uploaded;
    },
    [loadCatalog, loadPackages, onApplicationsChanged],
  );

  const removePackage = useCallback(
    async (packageId: string, uninstallInstalled = false) => {
      await applicationsApi.removePackage(packageId, uninstallInstalled);
      // A cascade uninstalls copies too, so the installed list is as stale as
      // the catalog afterwards.
      await Promise.all([loadCatalog(), loadPackages(), reload()]);
      onApplicationsChanged?.();
    },
    [loadCatalog, loadPackages, reload, onApplicationsChanged],
  );

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
      onApplicationsChanged?.();
    },
    [bindings, upsert, onApplicationsChanged],
  );

  const start = useCallback(
    async (appId: string) => {
      if (!bindings) return;
      upsert(await bindings.start(appId));
      onApplicationsChanged?.();
    },
    [bindings, upsert, onApplicationsChanged],
  );

  const stop = useCallback(
    async (appId: string) => {
      if (!bindings) return;
      upsert(await bindings.stop(appId));
      onApplicationsChanged?.();
    },
    [bindings, upsert, onApplicationsChanged],
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
      onApplicationsChanged?.();
    },
    [bindings, onApplicationsChanged],
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
    packages,
    managesPackages,
    uploadPackage,
    removePackage,
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
  isAdmin: boolean,
  onApplicationsChanged?: ApplicationsChanged,
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
    managesPackages: isAdmin,
    bindings,
    onApplicationsChanged,
  });
}

/** Applications scoped to a single project. */
export function useProjectApplications(
  project: ProjectMeta | null,
  enabled: boolean,
  isAdmin: boolean,
  onApplicationsChanged?: ApplicationsChanged,
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
    managesPackages: isAdmin,
    bindings,
    onApplicationsChanged,
    projectId: id ?? undefined,
  });
}
