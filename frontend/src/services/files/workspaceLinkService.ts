import { fileService } from "./fileService.ts";
import type { FileOpenRequest } from "../../models/files.ts";

const containerWorkspacePath = "/workspace";
const workspaceSegment = "/workspace";

export function internalPathOpenUrl(href: string, context: WorkspaceLinkContext = {}): string | null {
  const ref = splitLineReference(stripPathSuffix(href));
  const path = normalizeAbsolutePath(ref.path);
  if (!path) return null;
  if (!isContainerWorkspacePath(path) && !isHostWorkspacePath(path)) return null;

  if (context.chatId && isBrowserMediaPath(path)) {
    const params = new URLSearchParams({ path: refToString(path, ref) });
    return `/api/chats/${encodeURIComponent(context.chatId)}/media-open?${params.toString()}`;
  }

  const workspaceRoot = workspaceRootFromCwd(context.cwd);
  if (isContainerWorkspacePath(path)) {
    if (!workspaceRoot) return null;
    return context.openFileUrl?.({ cwd: workspaceRoot, path, line: ref.line, column: ref.column })
      ?? downloadUrl(context.chatId, path);
  }

  const marker = workspaceMarkerIndex(path);
  if (!workspaceRoot || (path !== workspaceRoot && !path.startsWith(`${workspaceRoot}/`))) return null;
  const containerPath = path.slice(marker);
  return context.openFileUrl?.({
    cwd: workspaceRootFromCwd(path) || workspaceRoot,
    path: containerPath,
    line: ref.line,
    column: ref.column,
  }) ?? downloadUrl(context.chatId, containerPath);
}

function downloadUrl(chatId: string | undefined, path: string): string | null {
  if (!chatId || !path.startsWith("/workspace/")) return null;
  const relativePath = path.slice("/workspace/".length);
  return `/api/chats/${encodeURIComponent(chatId)}/files/download?path=${encodeURIComponent(relativePath)}`;
}

interface PathLineReference {
  path: string;
  line?: number;
  column?: number;
}

function refToString(path: string, ref: PathLineReference): string {
  if (!ref.line) return path;
  if (ref.column) return `${path}:${ref.line}:${ref.column}`;
  return `${path}:${ref.line}`;
}

function splitLineReference(raw: string): PathLineReference {
  const trimmed = raw.trim();
  const lastColon = trimmed.lastIndexOf(":");
  if (lastColon < 0) return { path: trimmed };
  const lastPart = trimmed.slice(lastColon + 1);
  const lastNumber = Number(lastPart);
  if (!Number.isInteger(lastNumber) || lastNumber <= 0) return { path: trimmed };

  const beforeLast = trimmed.slice(0, lastColon);
  const secondColon = beforeLast.lastIndexOf(":");
  if (secondColon >= 0) {
    const maybeLine = Number(beforeLast.slice(secondColon + 1));
    if (Number.isInteger(maybeLine) && maybeLine > 0) {
      return { path: beforeLast.slice(0, secondColon), line: maybeLine, column: lastNumber };
    }
  }
  return { path: beforeLast, line: lastNumber };
}

export interface WorkspaceLinkContext {
  chatId?: string;
  cwd?: string;
  openFileUrl?: (request: FileOpenRequest) => string | null;
}

function isBrowserMediaPath(path: string): boolean {
  return fileService.viewableMediaKind(path) !== null;
}

function workspaceRootFromCwd(cwd?: string): string {
  const path = normalizeAbsolutePath(cwd || "");
  if (!path) return "";
  const marker = workspaceMarkerIndex(path);
  if (marker >= 0) return path.slice(0, marker + workspaceSegment.length);
  return path;
}

function isContainerWorkspacePath(path: string): boolean {
  return path === containerWorkspacePath || path.startsWith(`${containerWorkspacePath}/`);
}

function isHostWorkspacePath(path: string): boolean {
  return workspaceMarkerIndex(path) >= 0;
}

function workspaceMarkerIndex(path: string): number {
  if (path === workspaceSegment) return 0;
  if (path.endsWith(workspaceSegment)) return path.length - workspaceSegment.length;
  return path.indexOf(`${workspaceSegment}/`);
}

function normalizeAbsolutePath(path: string): string {
  const trimmed = path.trim();
  if (!trimmed.startsWith("/")) return "";
  return trimmed.replace(/\/{2,}/g, "/").replace(/\/+$/, "") || "/";
}

function stripPathSuffix(href: string): string {
  const hashIndex = href.indexOf("#");
  const queryIndex = href.indexOf("?");
  const cut = [hashIndex, queryIndex].filter((index) => index >= 0).sort((a, b) => a - b)[0];
  return cut === undefined ? href : href.slice(0, cut);
}
