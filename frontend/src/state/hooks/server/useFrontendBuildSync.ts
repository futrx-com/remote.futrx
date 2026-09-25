import { useEffect } from "preact/hooks";
import { FRONTEND_BUILD } from "../../../config/build.ts";
import { SESSION_STORAGE_KEYS } from "../../../config/storageKeys.ts";
import { frontendBuildStore } from "../../stores/server/frontendBuildStore.ts";
import { frontendBuildReloadState } from "./frontendBuildReloadState.ts";

/**
 * Reloads the page onto the frontend the server serves once a deploy has
 * replaced the one it is running, so nobody stays on an old build or has to
 * know to reload. The page asks when it opens, when it comes back into view,
 * when the network returns, and once a minute while it is on screen.
 */
export function useFrontendBuildSync(): void {
  useEffect(() => {
    const running =
      document.querySelector<HTMLMetaElement>(`meta[name="${FRONTEND_BUILD.metaName}"]`)
        ?.content || null;
    // Only production builds are stamped; the Vite dev server updates itself.
    if (!running) return;

    let reloading = false;

    const settle = () => {
      if (reloading) return;
      const { served } = frontendBuildStore.getState();
      if (!served) return;
      const reloadedFor = readReloadedFor();
      if (!frontendBuildReloadState.shouldReload({ running, served, reloadedFor })) return;
      // Unrecorded, the next page could not tell a stale cache from an update
      // and would reload forever; staying on the old build is the lesser harm.
      if (!writeReloadedFor(served)) return;
      reloading = true;
      window.location.reload();
    };

    const check = () => {
      void frontendBuildStore.getState().check();
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") check();
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) check();
    };

    // A newer build, heard here or by the updates screen.
    const unsubscribe = frontendBuildStore.subscribe(settle);
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("pageshow", onPageShow);
    window.addEventListener("online", check);
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") check();
    }, FRONTEND_BUILD.checkIntervalMs);
    check();

    return () => {
      unsubscribe();
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("pageshow", onPageShow);
      window.removeEventListener("online", check);
      window.clearInterval(interval);
    };
  }, []);
}

function readReloadedFor(): string | null {
  try {
    return sessionStorage.getItem(SESSION_STORAGE_KEYS.frontendBuildReload);
  } catch {
    return null;
  }
}

function writeReloadedFor(build: string): boolean {
  try {
    sessionStorage.setItem(SESSION_STORAGE_KEYS.frontendBuildReload, build);
    return true;
  } catch {
    return false;
  }
}
