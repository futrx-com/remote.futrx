import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type { FleetResourcesView, FleetSettings } from "../models/resources";

/**
 * Admin-only fleet resource policy. The PUT accepts a partial document: the
 * server keeps any field the form did not submit.
 */
export const adminResourcesApi = {
  fetch: () => requestJson<FleetResourcesView>("GET", API_ROUTES.adminResources),

  save: (settings: Partial<FleetSettings>) =>
    requestJson<FleetResourcesView>("PUT", API_ROUTES.adminResources, settings),
};
