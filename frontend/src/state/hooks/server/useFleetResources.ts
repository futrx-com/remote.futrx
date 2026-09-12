import { useCallback, useEffect, useState } from "preact/hooks";
import { adminResourcesApi } from "../../../api/adminResourcesApi";
import type { FleetResourcesView, FleetSettings } from "../../../models/resources";

/**
 * Admin-only fleet resource policy. Loaded on demand (the Settings tab), not
 * polled: the numbers only move when an operator changes them or a container
 * starts, and both paths already refresh the view.
 */
export function useFleetResources(enabled: boolean) {
  const [view, setView] = useState<FleetResourcesView | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setView(await adminResourcesApi.fetch());
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  const save = useCallback(async (settings: Partial<FleetSettings>) => {
    setSaving(true);
    try {
      setView(await adminResourcesApi.save(settings));
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
      throw cause;
    } finally {
      setSaving(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  return { view, loading, saving, error, refresh, save };
}
