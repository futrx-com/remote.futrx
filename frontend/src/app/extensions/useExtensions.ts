import { useEffect } from "preact/hooks";
import { extensionHost } from "./extensionHost";

/**
 * Keeps the loaded extensions in step with what is installed.
 *
 * The host is synced once on sign-in and again whenever the tab is shown,
 * because nothing pushes installs to a browser: an app uninstalled elsewhere
 * would otherwise keep drawing here for the life of the tab, calling a plugin
 * that is no longer running.
 */
export function useExtensions(enabled: boolean): void {
  useEffect(() => {
    if (!enabled) return;
    void extensionHost.sync();
    const resync = () => {
      if (document.visibilityState === "visible") void extensionHost.sync();
    };
    document.addEventListener("visibilitychange", resync);
    return () => document.removeEventListener("visibilitychange", resync);
  }, [enabled]);
}
