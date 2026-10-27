import { resolveFileOpener } from "../../../services/files/resolveFileOpener.ts";
import { useCallback, useEffect, useState } from "preact/hooks";
import { fileOpenerStore } from "../../stores/files/fileOpenerStore.ts";
import type { FileOpenRequest } from "../../../models/files.ts";
import { extensionStore } from "../../stores/extensions/extensionStore.ts";

// Refresh rendered links when an installed application changes its opener.
export function useWorkspaceFileUrl(): (request: FileOpenRequest) => string | null {
  const [openerVersion, setVersion] = useState(0);
  useEffect(() => {
    const refresh = () => setVersion((version) => version + 1);
    const unsubscribeOpener = fileOpenerStore.subscribe(refresh);
    const unsubscribeProject = extensionStore.subscribe((current, previous) => {
      if (current.activeProjectId !== previous.activeProjectId) refresh();
    });
    return () => {
      unsubscribeOpener();
      unsubscribeProject();
    };
  }, []);
  return useCallback(
    (request: FileOpenRequest) => resolveFileOpener(fileOpenerStore.getState().forProject(extensionStore.getState().activeProjectId ?? undefined), request),
    [openerVersion],
  );
}
