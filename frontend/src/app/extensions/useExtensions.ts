import { useEffect } from "preact/hooks";
import { extensionHost } from "./extensionHost";

/**
 * Binds the extension host to the signed-in session: it syncs while watched
 * and stops when this unmounts. When it re-syncs is the host's own policy.
 */
export function useExtensions(enabled: boolean): void {
  useEffect(() => (enabled ? extensionHost.watch() : undefined), [enabled]);
}
