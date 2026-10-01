// Helper to detect whether a code block is a mermaid diagram. Pulled out so
// Markdown rendering and the standalone component share one definition.
const MERMAID_LANGS = new Set(["mermaid", "mmd"]);

export function isMermaidLanguage(lang: string | undefined): boolean {
  if (!lang) return false;
  return MERMAID_LANGS.has(lang.trim().toLowerCase());
}

// Browsers word a failed lazy chunk differently: Chromium "Failed to fetch
// dynamically imported module", Safari "Importing a module script failed",
// Firefox "error loading dynamically imported module". Any of them means the
// page references assets the server no longer (or could not) hand back, which
// a reload fixes; a mermaid parse error does not.
const CHUNK_LOAD_ERROR =
  /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module/i;

export function isChunkLoadError(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err ?? "");
  return CHUNK_LOAD_ERROR.test(message);
}
