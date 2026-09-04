import { useEffect } from "preact/hooks";
import { extensionHost } from "./extensionHost";

export function useExtensions(enabled: boolean): void {
  useEffect(() => {
    if (enabled) void extensionHost.sync();
  }, [enabled]);
}
