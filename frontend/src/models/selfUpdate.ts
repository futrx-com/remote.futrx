export type SelfUpdateKind = "application" | "infrastructure";

export interface SelfUpdateCheck {
  checkedAt: number;
  latestTag?: string;
  updateAvailable: boolean;
  updateKind?: SelfUpdateKind;
  error?: string;
}

export interface SelfUpdateRun {
  state: "running" | "succeeded" | "failed";
  target: string;
  updateKind?: SelfUpdateKind;
  startedAt: number;
  startedBy?: string;
  finishedAt?: number;
  exitCode?: number;
  log?: string;
}

export interface SelfUpdateStatus {
  currentVersion: string;
  lastCheck?: SelfUpdateCheck | null;
  run?: SelfUpdateRun | null;
}
