import { requestJson } from "../apiRequest";
import type {
  ContainerLimits,
  ProjectContainerInfo,
  ProjectMeta,
} from "../../models/project";
import type { ProjectResources } from "../../models/resources";
import { API_ROUTES } from "../../config/routes";

export const projectContainerApi = {
  // force skips the aggregate host-memory guard. Admin only, and rejected
  // server-side for anyone else.
  start: (id: string, force = false) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.start(id, force), {}),

  stop: (id: string) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.stop(id), {}),

  restart: (id: string) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.restart(id), {}),

  fetchContainerInfo: (id: string) =>
    requestJson<ProjectContainerInfo>(
      "GET",
      API_ROUTES.projects.container(id)
    ),

  fetchProjectResources: (id: string) =>
    requestJson<ProjectResources>("GET", API_ROUTES.projects.resources(id)),

  setProjectResources: (id: string, limits: ContainerLimits) =>
    requestJson<ProjectResources>("PUT", API_ROUTES.projects.resources(id), limits),

  repairNetwork: (id: string) =>
    requestJson<ProjectContainerInfo>(
      "POST",
      API_ROUTES.projects.repairNetwork(id),
      {}
    ),
};
