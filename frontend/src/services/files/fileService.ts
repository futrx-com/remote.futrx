import type { FileCategory, MediaKind } from "../../models/files.ts";

export type FileOpenAction =
  | { action: "media"; kind: MediaKind }
  | { action: "ide" }
  | { action: "download" };

// What a filename means to the app: the kind of thing it is, whether the
// viewer can show it, what a click on it should do, and how to write its size.
class FileService {
  private readonly categoryByExtension: Record<string, FileCategory> = {
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

  // Mirrors the backend's supported inline media types (workspacefiles
  // mediaTypes): only these extensions render through the media-open endpoint.
  private readonly mediaKindByExtension: Record<string, MediaKind> = {
    avif: "image", bmp: "image", gif: "image", ico: "image", jpeg: "image",
    jpg: "image", png: "image", svg: "image", tif: "image", tiff: "image", webp: "image",
    m4v: "video", mov: "video", mp4: "video", ogv: "video", webm: "video",
    aac: "audio", flac: "audio", m4a: "audio", mp3: "audio", oga: "audio",
    ogg: "audio", opus: "audio", wav: "audio",
    pdf: "pdf",
  };

  category(name: string): FileCategory {
    return this.categoryByExtension[this.extension(name)] ?? "text";
  }

  /** The in-app viewer kind for a filename, or null when the browser (and the
   *  media-open endpoint) cannot render it inline. */
  viewableMediaKind(name: string): MediaKind | null {
    const extension = this.extension(name);
    return extension ? this.mediaKindByExtension[extension] ?? null : null;
  }

  /** What a click on a file should do: render viewable media in the in-app
   *  viewer, download what neither the browser nor the IDE can display
   *  (archives, unsupported media), and open everything else in the IDE. */
  openAction(name: string): FileOpenAction {
    const kind = this.viewableMediaKind(name);
    if (kind) return { action: "media", kind };
    const category = this.category(name);
    if (category === "archive" || category === "image" || category === "video" || category === "audio") {
      return { action: "download" };
    }
    return { action: "ide" };
  }

  /** File-tree sizes, which have a column to themselves: spaced unit, one
   *  decimal below ten ("980 B", "4.2 KB", "126 MB"). */
  formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let value = bytes / 1024;
    let i = 0;
    while (value >= 1024 && i < units.length - 1) {
      value /= 1024;
      i++;
    }
    return `${value.toFixed(value < 10 ? 1 : 0)} ${units[i]}`;
  }

  /** Attachment chips, which share a line with the filename and a progress
   *  label. Deliberately narrower than `formatBytes`: no space below a
   *  kilobyte, whole kilobytes, and it stops at MB ("980B", "4 KB", "1.2 MB"). */
  formatBytesCompact(bytes: number): string {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }

  /** The parent directory path of a workspace-relative path ("" = workspace root). */
  parentDir(path: string): string {
    const slash = path.lastIndexOf("/");
    return slash < 0 ? "" : path.slice(0, slash);
  }

  /** The lowercased extension, or "" when the name carries none. */
  private extension(name: string): string {
    const dot = name.lastIndexOf(".");
    return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
  }
}

export const fileService = new FileService();
