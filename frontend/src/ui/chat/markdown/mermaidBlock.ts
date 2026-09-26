// Helper to detect whether a code block is a mermaid diagram. Pulled out so
// Markdown rendering and the standalone component share one definition.
const MERMAID_LANGS = new Set(["mermaid", "mmd"]);

export function isMermaidLanguage(lang: string | undefined): boolean {
  if (!lang) return false;
  return MERMAID_LANGS.has(lang.trim().toLowerCase());
}
