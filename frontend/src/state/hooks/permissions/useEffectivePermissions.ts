import { useEffect, useState } from "preact/hooks";
import type { EffectivePermissionSnapshot } from "../../../models/rbac";
import { usePermissionLookup } from "../../context/PermissionLookupContext";
import { EFFECTIVE_PERMISSIONS_REFRESH_MS, PERMISSIONS_CHANGED_EVENT } from "../../../config/constants/permissions";

// UI decisions are hints only: owning services re-check every operation.
// Hide actions while loading or after a failed refresh. Ignore stale responses.
export function useEffectivePermissions(projectId?: string) {
  const lookup = usePermissionLookup();
  const [snapshot, setSnapshot] = useState<EffectivePermissionSnapshot>({ values: {} });
  useEffect(() => {
    let disposed = false;
    let sequence = 0;
    const refresh = async () => {
      const current = ++sequence;
      const next = await lookup.effectiveSnapshot(projectId);
      if (!disposed && current === sequence) setSnapshot(next);
    };
    void refresh();
    window.addEventListener("focus", refresh);
    window.addEventListener(PERMISSIONS_CHANGED_EVENT, refresh);
    const timer = window.setInterval(refresh, EFFECTIVE_PERMISSIONS_REFRESH_MS);
    return () => {
      disposed = true;
      window.clearInterval(timer);
      window.removeEventListener("focus", refresh);
      window.removeEventListener(PERMISSIONS_CHANGED_EVENT, refresh);
    };
  }, [projectId, lookup]);
  return snapshot.scope === projectId ? snapshot.values : {};
}
