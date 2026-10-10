import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { ApiError } from "../../../api/apiError";
import { applicationsApi } from "../../../api/applicationsApi";
import { API_RESPONSE_STATUS } from "../../../config/api";
import { projectApi } from "../../../api/projectApi";
import type {
  AppCredentials,
  AppApplication,
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
  catalog: AppApplication[];
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
  /**
   * Why the package list is empty, when it is empty because listing it failed.
   * A server built without a package store is not that: it has no uploaded
   * applications, and says so, so it leaves this unset.
   */
  packagesError?: string;
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

/**
 * Notified once this controller has established what is installed — after a
 * load as well as after an install, uninstall, or package change. The consumer
 * that matters is the extension host: it holds derived state, the `ui/`
 * modules it has loaded, and a change made anywhere else reaches it through no
 * other signal.
 */
type ApplicationsSettled = () => void;

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
  onApplicationsSettled?: ApplicationsSettled;
  projectId?: string;
}

function useApplicationsCore({
  scope,
  enabled,
  managesPackages,
  bindings,
  onApplicationsSettled,
  projectId,
}: CoreOptions): ApplicationsController {
  const [catalog, setCatalog] = useState<AppApplication[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [instances, setInstances] = useState<AppInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [packages, setPackages] = useState<AppPackage[]>([]);
  const [packagesError, setPackagesError] = useState<string | undefined>();
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

  const loadPackages = useCallback(async () => {
    if (!enabled || !managesPackages) return;
    try {
      const data = await applicationsApi.packages();
      setPackages(data ?? []);
      setPackagesError(undefined);
    } catch (err) {
      // A server built without a package store answers 503 here. That is not
      // an error to show: it simply has no uploaded applications to list.
      //
      // Anything else — a network failure, a 500 — leaves the catalog's
      // contents unknown, and an empty list is then a claim rather than an
      // answer. Reporting the failure is what keeps an operator from
      // re-uploading a package that is already there.
      setPackages([]);
      setPackagesError(packageStoreAbsent(err) ? undefined : (err as Error).message);
    }
  }, [enabled, managesPackages]);

  useEffect(() => {
    let cancelled = false;
    if (!enabled) return;
    void (async () => {
      await Promise.all([loadCatalog(), reload(), loadPackages()]);
      if (cancelled) return;
      // Opening a surface reconciles the extension host, not just changing
      // something on it. A change made anywhere else — another tab, another
      // administrator, a server that restarted without the application — reaches
      // this tab through no other path, and without this the surface can list
      // no installed apps while still rendering an uninstalled one's panel.
      notifySettled();
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, loadCatalog, reload, loadPackages, notifySettled]);

  // Uploading and removing both change what the catalog holds, so both end by
  // reloading it — the new card has to appear without a page refresh, and a
  // removed one has to stop offering an install that would now fail.
  const uploadPackage = useCallback(
    async (file: File) => {
      const uploaded = await applicationsApi.uploadPackage(file);
      // Replacing a package at a new version re-runs the install script in
      // every container that already holds it, and rewrites each copy's
      // recorded version and status — the upload response lists exactly which.
      // The installed list is therefore as stale as the catalog afterwards.
      await Promise.all([loadCatalog(), loadPackages(), reload()]);
      notifySettled();
      return uploaded;
    },
    [loadCatalog, loadPackages, reload, notifySettled],
  );

  const removePackage = useCallback(
    async (packageId: string, uninstallInstalled = false) => {
      await applicationsApi.removePackage(packageId, uninstallInstalled);
      // A cascade uninstalls copies too, so the installed list is as stale as
      // the catalog afterwards.
      await Promise.all([loadCatalog(), loadPackages(), reload()]);
      notifySettled();
    },
    [loadCatalog, loadPackages, reload, notifySettled],
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
    packages,
    packagesError,
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

/**
 * Whether the server simply has no package store, rather than having failed to
 * answer. It is the one reason an empty list is the truth.
 */
function packageStoreAbsent(err: unknown): boolean {
  return err instanceof ApiError && err.status === API_RESPONSE_STATUS.serviceUnavailable;
}

/** Global (server-wide) applications; admin-only. */
export function useGlobalApplications({
  enabled,
  managesPackages,
  onApplicationsSettled,
}: {
  enabled: boolean;
  managesPackages: boolean;
  onApplicationsSettled?: ApplicationsSettled;
}): ApplicationsController {
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
    managesPackages,
    bindings,
    onApplicationsSettled,
  });
}

/** Applications scoped to a single project. */
export function useProjectApplications({
  project,
  enabled,
  managesPackages,
  onApplicationsSettled,
}: {
  project: ProjectMeta | null;
  enabled: boolean;
  managesPackages: boolean;
  onApplicationsSettled?: ApplicationsSettled;
}): ApplicationsController {
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
    managesPackages,
    bindings,
    onApplicationsSettled,
    projectId: id ?? undefined,
  });
}
