export function fullResponseErrorMessage(cause: unknown): string {
  if (cause instanceof Error && cause.message) return cause.message;
  if (typeof cause === "string" && cause.trim()) return cause;
  return "Failed to load the full response.";
}

export function fullResponseLabel(loading: boolean, bytes?: number): string {
  if (loading) return "Loading full response…";
  return `Load full response${bytes ? ` (${formatBytes(bytes)})` : ""}`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
