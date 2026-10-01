import { requestJson } from "./apiRequest";

export type PermissionScope = { kind: "platform" | "project"; id?: string };
export type PermissionRule = { Permission: string; Effect: "allow" | "deny" };
export type PermissionDefinition = { Key: string; Description: string; Scopes: PermissionScope["kind"][]; Baseline: string; Delegable: boolean };
export type PermissionAssignment = PermissionRule & { ID: string; UserEmail: string; Scope: PermissionScope };
export type PermissionRole = { ID: string; Name: string; Description: string; Rules: PermissionRule[] };
export type PermissionBinding = { ID: string; RoleID: string; UserEmail: string; Scope: PermissionScope };
export type PermissionPolicy = { definitions: PermissionDefinition[]; assignments: PermissionAssignment[] | null; roles: PermissionRole[] | null; bindings: PermissionBinding[] | null };
const base = "/api/permissions";
export const permissionsApi = {
  policy: () => requestJson<PermissionPolicy>("GET", base),
  effective: (projectId?: string) => requestJson<Record<string, boolean>>("GET", `${base}/effective${projectId ? `?projectId=${encodeURIComponent(projectId)}` : ""}`),
  assign: (input: Omit<PermissionAssignment, "ID">) => requestJson("PUT", `${base}/assignments`, input),
  removeAssignment: ({ UserEmail, Permission, Scope }: PermissionAssignment) => requestJson("DELETE", `${base}/assignments`, { UserEmail, Permission, Scope }),
  saveRole: (input: Omit<PermissionRole, "ID">, id?: string) => requestJson(id ? "PUT" : "POST", `${base}/roles${id ? `/${encodeURIComponent(id)}` : ""}`, input),
  removeRole: (id: string) => requestJson("DELETE", `${base}/roles/${encodeURIComponent(id)}`),
  bind: (input: Omit<PermissionBinding, "ID">) => requestJson("PUT", `${base}/bindings`, input),
  unbind: ({ UserEmail, RoleID, Scope }: PermissionBinding) => requestJson("DELETE", `${base}/bindings`, { UserEmail, RoleID, Scope }),
};
