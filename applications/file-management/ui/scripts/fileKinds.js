const CATEGORY_BY_EXTENSION = {
  png: "image", jpg: "image", jpeg: "image", gif: "image", webp: "image",
  svg: "image", avif: "image", bmp: "image", ico: "image", heic: "image",
  mp4: "video", mov: "video", webm: "video", mkv: "video", avi: "video", m4v: "video",
  mp3: "audio", wav: "audio", flac: "audio", ogg: "audio", m4a: "audio", aac: "audio",
  pdf: "pdf",
  zip: "archive", tar: "archive", gz: "archive", tgz: "archive", rar: "archive", "7z": "archive",
  ts: "code", tsx: "code", js: "code", jsx: "code", go: "code", py: "code", rs: "code",
  java: "code", c: "code", cpp: "code", h: "code", css: "code", html: "code", sh: "code", rb: "code",
  json: "data", csv: "data", yaml: "data", yml: "data", xml: "data", toml: "data",
  sql: "data", db: "data", sqlite: "data",
  txt: "text", md: "text", log: "text",
};

// Kept in sync with backend/workspace/service.go. Browser-unsupported media
// deliberately falls back to download rather than being sent to the IDE.
const MEDIA_KIND_BY_EXTENSION = {
  avif: "image", bmp: "image", gif: "image", ico: "image", jpeg: "image",
  jpg: "image", png: "image", svg: "image", tif: "image", tiff: "image", webp: "image",
  m4v: "video", mov: "video", mp4: "video", ogv: "video", webm: "video",
  aac: "audio", flac: "audio", m4a: "audio", mp3: "audio", oga: "audio",
  ogg: "audio", opus: "audio", wav: "audio",
  pdf: "pdf",
};

export function extension(name) {
  const dot = name.lastIndexOf(".");
  return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
}

export function category(name) {
  return CATEGORY_BY_EXTENSION[extension(name)] ?? "text";
}

export function viewableMediaKind(name) {
  return MEDIA_KIND_BY_EXTENSION[extension(name)] ?? null;
}

export function openAction(name) {
  const kind = viewableMediaKind(name);
  if (kind) return { action: "media", kind };
  const fileCategory = category(name);
  if (["archive", "image", "video", "audio"].includes(fileCategory)) {
    return { action: "download" };
  }
  return { action: "ide" };
}

export function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[index]}`;
}

export function parentDir(path) {
  const slash = path.lastIndexOf("/");
  return slash < 0 ? "" : path.slice(0, slash);
}

export function openTitle(name) {
  const target = openAction(name);
  if (target.action === "media") return `View ${name}`;
  if (target.action === "ide") return `Open ${name} in IDE`;
  return `Download ${name}`;
}
