import type { MediaKind } from "../../models/files.ts";

// Shared file-name policy retained for chat attachments and inline media links.
// The File Management application owns browsing, categories, and click routing.
class FileService {
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

  viewableMediaKind(name: string): MediaKind | null {
    const extension = this.extension(name);
    return extension ? this.mediaKindByExtension[extension] ?? null : null;
  }

  formatBytesCompact(bytes: number): string {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }

  private extension(name: string): string {
    const dot = name.lastIndexOf(".");
    return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
  }
}

export const fileService = new FileService();
