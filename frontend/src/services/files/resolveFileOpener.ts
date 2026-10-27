import type { FileOpener, FileOpenRequest } from "../../models/files.ts";

// Registration order decides which available editor handles the request.
export function resolveFileOpener(openers: Iterable<FileOpener>, request: FileOpenRequest): string | null {
  for (const open of openers) {
    try {
      const url = open(request);
      if (url) return url;
    } catch (error) {
      console.error("[extensions] file opener failed", error);
    }
  }
  return null;
}
