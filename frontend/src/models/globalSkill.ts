// A global skill is one directory of the platform-wide library that every
// project sees in addition to its own workspace skills. `files` is populated
// only by the single-skill read; the list view carries `fileNames` instead.
export interface GlobalSkill {
  name: string;
  title?: string;
  description?: string;
  alwaysOn: boolean;
  updatedAt?: number;
  fileNames?: string[];
  files?: Record<string, string>;
}

export interface GlobalSkillInput {
  name: string;
  files: Record<string, string>;
  alwaysOn?: boolean;
}

export interface GlobalSkillUpdate {
  files?: Record<string, string>;
  alwaysOn?: boolean;
}

export interface GlobalSkillImport {
  projectId: string;
  skill: string;
  name?: string;
  alwaysOn?: boolean;
}
