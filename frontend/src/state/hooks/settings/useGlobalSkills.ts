import { useCallback, useEffect, useState } from "preact/hooks";
import { globalSkillApi } from "../../../api/agents/globalSkillApi";
import type { GlobalSkill } from "../../../models/globalSkill";
import { globalSkillsState } from "../../settings/globalSkillsState";
import type { GlobalSkillDraft } from "../../settings/globalSkillsState";

export interface GlobalSkillLibrary {
  skills: GlobalSkill[] | null;
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  read: (name: string) => Promise<GlobalSkill>;
  create: (draft: GlobalSkillDraft) => Promise<void>;
  save: (name: string, draft: GlobalSkillDraft) => Promise<void>;
  remove: (name: string) => Promise<void>;
  setAlwaysOn: (name: string, alwaysOn: boolean) => Promise<void>;
  importFromProject: (
    projectId: string,
    skill: string,
    name?: string
  ) => Promise<void>;
}

// useGlobalSkills owns the admin library's remote state. Every mutation
// folds the server's response back into the local list through
// globalSkillsState so ordering stays identical to a fresh load.
export function useGlobalSkills(enabled: boolean): GlobalSkillLibrary {
  const [skills, setSkills] = useState<GlobalSkill[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSkills(await globalSkillApi.list());
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void reload();
  }, [enabled, reload]);

  const read = useCallback((name: string) => globalSkillApi.get(name), []);

  const create = useCallback(async (draft: GlobalSkillDraft) => {
    const created = await globalSkillApi.create({
      name: globalSkillsState.normalizeName(draft.name),
      files: globalSkillsState.buildFiles(draft),
      alwaysOn: draft.alwaysOn,
    });
    setSkills((current) => globalSkillsState.upsert(current ?? [], created));
  }, []);

  const save = useCallback(async (name: string, draft: GlobalSkillDraft) => {
    const updated = await globalSkillApi.update(name, {
      files: globalSkillsState.buildFiles(draft),
      alwaysOn: draft.alwaysOn,
    });
    setSkills((current) => globalSkillsState.upsert(current ?? [], updated));
  }, []);

  const remove = useCallback(async (name: string) => {
    await globalSkillApi.remove(name);
    setSkills((current) => globalSkillsState.remove(current ?? [], name));
  }, []);

  const setAlwaysOn = useCallback(async (name: string, alwaysOn: boolean) => {
    const updated = await globalSkillApi.update(name, { alwaysOn });
    setSkills((current) => globalSkillsState.upsert(current ?? [], updated));
  }, []);

  const importFromProject = useCallback(
    async (projectId: string, skill: string, name?: string) => {
      const imported = await globalSkillApi.importFromProject({
        projectId,
        skill,
        name,
      });
      setSkills((current) => globalSkillsState.upsert(current ?? [], imported));
    },
    []
  );

  return {
    skills,
    loading,
    error,
    reload,
    read,
    create,
    save,
    remove,
    setAlwaysOn,
    importFromProject,
  };
}
