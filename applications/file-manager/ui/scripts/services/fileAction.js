import { VIEWABLE, DOWNLOADED } from "../config/files.js";

function extension(path) {
  const name = path.slice(path.lastIndexOf("/") + 1);
  const dot = name.lastIndexOf(".");
  return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
}

/** "media", "editor" or "download", as Remote decides for a file click. */
export function fileAction(path) {
  const ext = extension(path);
  if (VIEWABLE.has(ext)) return "media";
  return DOWNLOADED.has(ext) ? "download" : "editor";
}
