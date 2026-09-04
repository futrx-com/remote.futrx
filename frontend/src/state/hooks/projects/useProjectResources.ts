import { useCallback, useEffect, useState } from "preact/hooks";
import { projectApi } from "../../../api/projectApi";
import type { ContainerLimits, ProjectMeta } from "../../../models/project";
import type { ProjectResources } from "../../../models/resources";

/**
 * Loads one project's resource envelope and saves overrides. Kept separate
 * from useProjectContainerInfo because the resources panel needs the fleet
 * policy and a live usage snapshot the plain inspection payload does not carry.
 */
export function useProjectResources(project: ProjectMeta | null, enabled: boolean) {
  const [data, setData] = useState<ProjectResources | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!project) {
      setData(null);
      return;
    }
    setLoading(true);
    try {
      setData(await projectApi.fetchProjectResources(project.id));
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, [project]);

  const save = useCallback(
    async (limits: ContainerLimits) => {
      if (!project) return;
      setSaving(true);
      try {
        setData(await projectApi.setProjectResources(project.id, limits));
        setError(null);
      } catch (cause) {
        setError((cause as Error).message);
        throw cause;
      } finally {
        setSaving(false);
      }
    },
    [project]
  );

  useEffect(() => {
    if (!enabled || !project) return;
    let cancelled = false;
    void (async () => {
      try {
        const next = await projectApi.fetchProjectResources(project.id);
        if (cancelled) return;
        setData(next);
        setError(null);
      } catch (cause) {
        if (!cancelled) setError((cause as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    setLoading(true);
    return () => {
      cancelled = true;
    };
  }, [enabled, project]);

  return { data, loading, saving, error, load, save };
}
