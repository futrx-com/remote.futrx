import { applicationOrigin } from "./applicationUrls.js";
import { CODE_SERVER } from "../config/files.js";

/** Code Server's address for a file, as its own opener builds it. */
export function codeServerFileUrl(cwd, path, origin = window.location.origin) {
  const url = applicationOrigin(cwd, CODE_SERVER.subdomain, origin);
  const location = /\/workspace(?:\/(.*))?$/.exec(cwd || "");
  if (!url || !location) return null;
  url.pathname = "/_static/remote-open.html";
  url.searchParams.set(
    "folder",
    location[1] ? `/workspace/${location[1]}` : "/workspace",
  );
  url.searchParams.set("file", path);
  return url.toString();
}

/** The link Scratch Tab recognises as "open this file in Scratch Tab". */
export function scratchFileUrl(chatId, path) {
  return `/api/chats/${encodeURIComponent(chatId)}/ide-open?scratch=1&path=${encodeURIComponent(path)}`;
}

export function createFileUrls(origin) {
  return {
    editor: (cwd, path) => codeServerFileUrl(cwd, path, origin),
    scratch: scratchFileUrl,
    media: (chatId, path) =>
      `/api/chats/${encodeURIComponent(chatId)}/media-open?path=${encodeURIComponent(path)}`,
    download: (chatId, relative) =>
      `/api/chats/${encodeURIComponent(chatId)}/files/download?path=${encodeURIComponent(relative)}`,
    absolute: (url) => new URL(url, origin).href,
  };
}
