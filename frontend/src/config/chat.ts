import type { ApprovalPolicy, SandboxPolicy } from "../models/chat";

export const APPROVAL_POLICY_OPTIONS: readonly {
  value: ApprovalPolicy;
  label: string;
}[] = [
  { value: "on-request", label: "Ask when needed" },
  { value: "untrusted", label: "Untrusted only" },
  { value: "never", label: "Never ask" },
];

export const SANDBOX_POLICY_OPTIONS: readonly {
  value: SandboxPolicy;
  label: string;
}[] = [
  { value: "workspaceWrite", label: "Workspace write" },
  { value: "readOnly", label: "Read only" },
  { value: "dangerFullAccess", label: "Full access" },
];

export function modelShortLabel(model?: string): string {
  return model || "auto";
}

export function providerDisplayLabel(provider?: string): string {
  if (!provider) return "Codex";
  const knownLabels: Record<string, string> = {
    antigravity: "Antigravity",
    claude: "Claude",
    codex: "Codex",
    kimi: "Kimi",
    minimax: "MiniMax",
  };
  return knownLabels[provider] ?? provider
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

/**
 * Where a chat's attachments land. The backend anchors them at
 * `<workspace root>/.uploads` and keeps that root stable on purpose — its own
 * comment in service.go says it does so "so the frontend can predict it
 * exactly". These are that prediction; they must not drift from it.
 */
export const CHAT_UPLOAD_PATHS = {
  /** Subdirectory isolating attachments from the source tree. */
  dirName: ".uploads",
  /** The stable root a project chat's uploads hang off, whatever its live cwd. */
  projectRoot: "/workspace",
} as const;
