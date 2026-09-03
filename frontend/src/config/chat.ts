export function modelShortLabel(model?: string): string {
  return model || "auto";
}

export function providerDisplayLabel(provider?: string): string {
  if (!provider) return "Codex";
  return provider.charAt(0).toUpperCase() + provider.slice(1);
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
