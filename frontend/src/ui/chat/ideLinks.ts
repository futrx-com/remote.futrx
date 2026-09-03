import { fileService } from "../../services/files/fileService.ts";

export const defaultWorkspacePath = "/opt/remote.futrx";

const containerWorkspacePath = "/workspace";
const workspaceSegment = "/workspace";

const projectsPrefix = "/var/lib/remote/projects/";

// Maps a host path under a project workspace
// (/var/lib/remote/projects/<slug>/workspace[/rel]) to the project slug and the
// equivalent in-container path (/workspace[/rel]). Returns null for non-project
// paths (e.g. the platform's own repo).
function projectSlugAndContainerPath(
  hostPath: string,
): { slug: string; containerPath: string } | null {
  const norm = normalizeAbsolutePath(hostPath);
  if (!norm.startsWith(projectsPrefix)) return null;
  const rest = norm.slice(projectsPrefix.length);
  const slash = rest.indexOf("/");
  if (slash <= 0) return null;
  const slug = rest.slice(0, slash);
  const after = rest.slice(slash + 1);
  if (after === "workspace") return { slug, containerPath: "/workspace" };
  if (after.startsWith("workspace/"))
    return { slug, containerPath: "/workspace/" + after.slice("workspace/".length) };
  return null;
}

export function buildIdeUrl(
  folderPath: string,
  filePath?: string,
  line?: number,
  column?: number,
): string {
  const folder = normalizeAbsolutePath(folderPath) || defaultWorkspacePath;
  const proj = projectSlugAndContainerPath(folder);
  const ideBaseUrl = currentIdeBaseUrl();
  if (proj) {
    // Per-container IDE: code.<installed-domain>/<slug>/.
    const url = new URL(`${proj.slug}/`, ideBaseUrl);
    url.searchParams.set("folder", proj.containerPath);
    if (filePath) {
      const f = projectSlugAndContainerPath(normalizeAbsolutePath(filePath));
      if (f && f.slug === proj.slug) {
        url.searchParams.set("payload", openFilePayload(url.host, f.containerPath, line, column));
      }
    }
    return url.toString();
  }
  // Fallback: non-project paths -> host code-server.
  const url = new URL(ideBaseUrl);
  url.searchParams.set("folder", folder);
  if (filePath) {
    url.searchParams.set(
      "payload",
      openFilePayload(url.host, normalizeAbsolutePath(filePath), line, column),
    );
  }
  return url.toString();
}

// openFilePayload builds the VS Code web-workbench payload that makes
// code-server open a file, optionally at line:column. code-server has no file
// query parameter; the workbench's payload contract (openFile + gotoLineMode)
// is what places the cursor. Mirrors the backend workspaceide service and was
// verified against code-server 4.121.0.
export function openFilePayload(
  host: string,
  absolutePath: string,
  line?: number,
  column?: number,
): string {
  const encodedPath = absolutePath.split("/").map(encodeURIComponent).join("/");
  let fileUri = `vscode-remote://${host}${encodedPath}`;
  const pairs: [string, string][] = [];
  if (line && line > 0) {
    fileUri += `:${line}`;
    if (column && column > 0) fileUri += `:${column}`;
    pairs.push(["openFile", fileUri], ["gotoLineMode", "true"]);
  } else {
    pairs.push(["openFile", fileUri]);
  }
  return JSON.stringify(pairs);
}

function currentIdeBaseUrl(): string {
  const url = new URL(location.origin);
  url.hostname = `code.${url.hostname}`;
  url.pathname = "/";
  return url.toString();
}

export function internalPathOpenUrl(href: string, context: IdeLinkContext = {}): string | null {
  const ref = splitLineReference(stripPathSuffix(href));
  const path = normalizeAbsolutePath(ref.path);
  if (!path) return null;
  if (!isContainerWorkspacePath(path) && !isHostWorkspacePath(path)) return null;

  if (context.chatId) {
    const params = new URLSearchParams({ path: refToString(path, ref) });
    const action = isBrowserMediaPath(path) ? "media-open" : "ide-open";
    return `/api/chats/${encodeURIComponent(context.chatId)}/${action}?${params.toString()}`;
  }

  const workspaceRoot = workspaceRootFromCwd(context.cwd);
  if (isContainerWorkspacePath(path)) {
    if (!workspaceRoot) return null;
    const rel = path.slice(containerWorkspacePath.length).replace(/^\/+/, "");
    const hostPath = rel ? `${workspaceRoot}/${rel}` : workspaceRoot;
    return buildIdeUrl(
      workspaceRoot,
      hostPath === workspaceRoot ? undefined : hostPath,
      ref.line,
      ref.column,
    );
  }

  const folder = workspaceRootFromCwd(path) || workspaceRoot || path;
  return buildIdeUrl(folder, path === folder ? undefined : path, ref.line, ref.column);
}

export function internalPathIdeUrl(href: string, context: IdeLinkContext = {}): string | null {
  return internalPathOpenUrl(href, context);
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

export interface IdeLinkContext {
  chatId?: string;
  cwd?: string;
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
