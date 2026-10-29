export const CODE_SERVER = { id: "code-server", subdomain: "code" };

export const VIEWABLE = new Set([
  "avif", "bmp", "gif", "ico", "jpeg", "jpg", "png", "svg", "tif", "tiff", "webp",
  "m4v", "mov", "mp4", "ogv", "webm",
  "aac", "flac", "m4a", "mp3", "oga", "ogg", "opus", "wav",
  "pdf",
]);
// Archives and media the browser cannot play: no editor opens these.
export const DOWNLOADED = new Set([
  "zip", "tar", "gz", "tgz", "rar", "7z",
  "heic", "mkv", "avi",
]);
