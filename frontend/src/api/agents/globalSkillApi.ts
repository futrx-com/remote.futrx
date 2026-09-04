import { requestJson } from "../apiRequest";
import type {
  GlobalSkill,
  GlobalSkillImport,
  GlobalSkillInput,
  GlobalSkillUpdate,
} from "../../models/globalSkill";
import { API_ROUTES } from "../../config/routes";

// Admin-only client for the platform-wide skills library. Every route is
// gated on the server; a non-admin session receives 403.
export const globalSkillApi = {
  list: () =>
    requestJson<GlobalSkill[]>("GET", API_ROUTES.globalSkills.collection),
  get: (name: string) =>
    requestJson<GlobalSkill>("GET", API_ROUTES.globalSkills.item(name)),
  create: (input: GlobalSkillInput) =>
    requestJson<GlobalSkill>("POST", API_ROUTES.globalSkills.collection, input),
  update: (name: string, input: GlobalSkillUpdate) =>
    requestJson<GlobalSkill>("PUT", API_ROUTES.globalSkills.item(name), input),
  remove: (name: string) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.globalSkills.item(name)),
  importFromProject: (input: GlobalSkillImport) =>
    requestJson<GlobalSkill>("POST", API_ROUTES.globalSkills.import, input),
};
