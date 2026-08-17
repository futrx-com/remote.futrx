/**
 * Project templates ("stack presets"). A template is the shared base image
 * plus a one-time in-container provisioning step, so choosing one at project
 * creation is the only place the stack is decided — it cannot be changed
 * afterwards.
 */
export interface ProjectTemplate {
  name: string;
  title: string;
  description: string;
  /** Stable key mapped to a glyph by the UI; unknown keys fall back. */
  icon: string;
  defaultPorts?: number[];
  /** True for the template assigned when none is requested. */
  default: boolean;
  /** False when the template installs nothing (the container starts at once). */
  provisions: boolean;
  /** Alias of the dedicated pre-built image, when the template declares one. */
  prebuiltImage?: string;
  /** True when that image is published on this host (fast first start). */
  prebuiltImageAvailable: boolean;
}

/** How far a project's one-time template provisioning has got. */
export type TemplateProvisionStatus =
  | "none"
  | "pending"
  | "running"
  | "done"
  | "failed";

export interface ProjectTemplateStatus {
  name: string;
  title?: string;
  status: TemplateProvisionStatus;
  error?: string;
  logPath?: string;
  startedAt?: number;
  finishedAt?: number;
}
