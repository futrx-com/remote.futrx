import { requestJson } from "../apiRequest";
import type {
  ContainerLimits,
  ProjectMeta,
} from "../../models/project";
import { API_ROUTES } from "../../config/routes";
import {
  normalizeProjectContainerInfo,
  type ProjectContainerInfoPayload,
} from "./projectContainerInfo.ts";

export const projectContainerApi = {
  start: (id: string) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.start(id), {}),

  stop: (id: string) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.stop(id), {}),

  restart: (id: string) =>
    requestJson<ProjectMeta>("POST", API_ROUTES.projects.restart(id), {}),

  fetchContainerInfo: (id: string) =>
    requestJson<ProjectContainerInfoPayload>(
      "GET",
      API_ROUTES.projects.container(id)
    ).then(normalizeProjectContainerInfo),

  setContainerLimits: (id: string, limits: ContainerLimits) =>
    requestJson<ProjectContainerInfoPayload>(
      "PUT",
      API_ROUTES.projects.limits(id),
      limits
    ).then(normalizeProjectContainerInfo),

  repairNetwork: (id: string) =>
    requestJson<ProjectContainerInfoPayload>(
      "POST",
      API_ROUTES.projects.repairNetwork(id),
      {}
    ).then(normalizeProjectContainerInfo),
};
