import {
  DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS,
  READ_TOOL_OUTPUT_PREVIEW_CHARS,
} from "../../../config/chat.ts";

export function shortPath(path: string | undefined): string {
  if (!path) return "";
  if (path.startsWith("/root/")) return "~" + path.slice(5);
  return path;
}

export function truncate(value: string, max: number): string {
  if (value.length <= max) return value;
  return value.slice(0, max) + `\n\n... (${value.length - max} more characters truncated)`;
}

export function toolOutputPreviewLimit(name: string): number | null {
  if (name === "Read") return READ_TOOL_OUTPUT_PREVIEW_CHARS;
  if (name === "Edit" || name === "MultiEdit" || name === "Write") return null;
  return DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS;
}
