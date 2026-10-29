import type { FileOpenRequest, FileOpenResult } from "../../models/files.ts";
import { API_ROUTES } from "../../config/routes.ts";
import { fileService } from "./fileService.ts";
import { resolveFileOpener } from "./resolveFileOpener.ts";
import { fileOpenerStore } from "../../state/stores/files/fileOpenerStore.ts";
import { mediaViewerStore } from "../../state/stores/media/mediaViewerStore.ts";

/** Shared file opening for application surfaces, chat links and media viewers. */
export function openWorkspaceFile(
  request: FileOpenRequest & { chatId: string },
): FileOpenResult {
  const relative = workspaceRelativePath(request);
  if (!relative) return "ignored";
  const name = relative.split("/").at(-1) || relative;
  const target = fileService.openAction(name);
  if (target.action === "media") {
    mediaViewerStore.getState().open({
      url: API_ROUTES.chats.mediaOpen(request.chatId, request.path),
      name,
      kind: target.kind,
    });
    return "media";
  }
  const url = workspaceFileEditorUrl(request);
  if (url) {
    if (claimedAsLink(url)) return "editor";
    return openTab(url) ? "editor" : "blocked";
  }
  window.location.assign(
    API_ROUTES.chats.fileDownload(request.chatId, relative),
  );
  return "download";
}

// Editors that run inside Remote's page (Scratch Tab) take over clicks on
// links to their URLs, as they do for chat file links. Offer the URL as a
// link click first; the editor claims it by stopping the event.
function claimedAsLink(url: string): boolean {
  const link = document.createElement("a");
  link.href = url;
  link.hidden = true;
  let unclaimed = false;
  link.addEventListener("click", (event) => {
    unclaimed = true;
    event.preventDefault();
  });
  document.body.append(link);
  try {
    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));
  } finally {
    link.remove();
  }
  return !unclaimed;
}

// window.open with "noopener" always returns null, so cut the opener by hand
// to still learn whether the browser blocked the tab.
function openTab(url: string): boolean {
  const tab = window.open("", "_blank");
  if (!tab) return false;
  tab.opener = null;
  tab.location.replace(url);
  return true;
}

/** The editor URL `openWorkspaceFile` would open in a new tab for this file,
 *  or null when it would show or download it instead. Lets a surface that
 *  must open the tab itself (an iframe, inside its own click) do so. */
export function workspaceFileEditorUrl(
  request: FileOpenRequest & { chatId: string },
): string | null {
  const relative = workspaceRelativePath(request);
  if (!relative) return null;
  const name = relative.split("/").at(-1) || relative;
  if (fileService.openAction(name).action !== "application") return null;
  return resolveFileOpener(
    fileOpenerStore.getState().forProject(request.projectId),
    request,
  );
}

// The path below /workspace, or null for anything outside it.
function workspaceRelativePath(
  request: FileOpenRequest & { chatId: string },
): string | null {
  if (!request.chatId || !request.path.startsWith("/workspace/")) return null;
  const relative = request.path.slice("/workspace/".length);
  if (
    !relative ||
    relative.split("/").some((part) => part === ".." || part === ".")
  )
    return null;
  return relative;
}
