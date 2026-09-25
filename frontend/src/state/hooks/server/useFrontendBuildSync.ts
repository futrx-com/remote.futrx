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
    let blurTimer: number | undefined;

    const settle = (opening: boolean) => {
      if (reloading) return;
      const { served, holds } = frontendBuildStore.getState();
      const decision = frontendBuildReloadState.decide({
        running,
        served,
        reloadedFor: readReloadedFor(),
        opening,
        hidden: document.visibilityState === "hidden",
        editing: isEditing(),
        held: holds > 0,
      });
      if (decision !== "reload" || !served) return;
      // Unrecorded, the next page could not tell a stale cache from an update
      // and would reload forever; staying on the old build is the lesser harm.
      if (!writeReloadedFor(served)) return;
      reloading = true;
      window.location.reload();
    };

    const check = (opening: boolean) => {
      void frontendBuildStore.getState().check().then(() => settle(opening));
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") check(true);
      else settle(false);
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) check(true);
    };
    const onOnline = () => check(false);
    // Focus has not landed anywhere yet while focusout runs; decide after it.
    const onFocusOut = () => {
      window.clearTimeout(blurTimer);
      blurTimer = window.setTimeout(() => settle(false), 0);
    };

    // A newer build or a released hold, from here or from another screen.
    const unsubscribe = frontendBuildStore.subscribe(() => settle(false));
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("pageshow", onPageShow);
    window.addEventListener("online", onOnline);
    window.addEventListener("focusout", onFocusOut);
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") check(false);
    }, FRONTEND_BUILD.checkIntervalMs);
    check(true);

    return () => {
      unsubscribe();
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("pageshow", onPageShow);
      window.removeEventListener("online", onOnline);
      window.removeEventListener("focusout", onFocusOut);
      window.clearInterval(interval);
      window.clearTimeout(blurTimer);
    };
  }, []);
}

function isEditing(): boolean {
  if (document.querySelector('[aria-modal="true"]')) return true;
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return false;
  return active.isContentEditable || active.matches("input, textarea, select");
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
