import { useEffect, useState } from "preact/hooks";
import { permissionsApi } from "../../../api/permissionsApi";

// UI decisions are hints only: owning services re-check every operation.
// Hide actions while loading or after a failed refresh. Ignore stale responses.
export function usePermissions(projectId?: string) {
  const [snapshot, setSnapshot] = useState<{ scope?: string; values: Record<string, boolean> }>({ values: {} });
  useEffect(() => {
    let disposed = false;
    let sequence = 0;
    const refresh = async () => {
      const current = ++sequence;
      try {
        const values = await permissionsApi.effective(projectId);
        if (!disposed && current === sequence) setSnapshot({ scope: projectId, values });
      } catch {
        if (!disposed && current === sequence) setSnapshot({ scope: projectId, values: {} });
      }
    };
    void refresh();
    window.addEventListener("focus", refresh);
    window.addEventListener("permissions-changed", refresh);
    const timer = window.setInterval(refresh, 30000);
    return () => { disposed = true; window.clearInterval(timer); window.removeEventListener("focus", refresh); window.removeEventListener("permissions-changed", refresh); };
  }, [projectId]);
  return snapshot.scope === projectId ? snapshot.values : {};
}
