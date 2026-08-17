import { requestJson } from "./apiRequest";
import type { ProjectTemplate } from "../models/template";
import { API_ROUTES } from "../config/routes";

export const templateApi = {
  list: () => requestJson<ProjectTemplate[]>("GET", API_ROUTES.templates),
};
