import { useCallback, useEffect, useState } from "preact/hooks";
import { selfUpdateApi } from "../../../api/selfUpdateApi";
import type { SelfUpdateReleaseNotes } from "../../../models/selfUpdate";

interface ReleaseNotesState {
  tag: string;
  notes: SelfUpdateReleaseNotes | null;
  loading: boolean;
  error: string | null;
}

export function useReleaseNotes(enabled: boolean, tag: string | undefined) {
  const [state, setState] = useState<ReleaseNotesState | null>(null);
  const [attempt, setAttempt] = useState(0);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  useEffect(() => {
    if (!enabled || !tag) return;
    let cancelled = false;
    setState({ tag, notes: null, loading: true, error: null });
    void selfUpdateApi.releaseNotes(tag).then(
      (notes) => {
        if (!cancelled) setState({ tag, notes, loading: false, error: null });
      },
      () => {
        if (!cancelled) setState({ tag, notes: null, loading: false, error: "Release notes could not be loaded." });
      },
    );
    return () => { cancelled = true; };
  }, [enabled, tag, attempt]);

  // A new release must never paint the previous tag's notes, even for the
  // render before its effect runs or when an older request finishes late.
  const current = enabled && state?.tag === tag ? state : null;
  return {
    notes: current?.notes ?? null,
    loading: enabled && !!tag && (!current || current.loading),
    error: current?.error ?? null,
    retry,
  };
}

export type ReleaseNotesController = ReturnType<typeof useReleaseNotes>;
