// Guarded access to the CSS Custom Highlight API.
//
// It styles live Ranges, so text can be marked inside already-rendered content
// without wrapping anything in <mark> — no reflow, nothing to unpick when the
// marks change, and no risk of splitting an element the renderer built. Not
// every browser has it (Safari before 17.2), and a missing highlight is never
// worth taking a feature down for, so every call resolves to a no-op there.
//
// Leaf service: it knows about the browser, never about what is being marked —
// callers name their own highlights and style them with `::highlight(name)`.

/** The slice of the Custom Highlight API this service uses. */
interface HighlightRegistry {
  set(name: string, highlight: unknown): void;
  delete(name: string): void;
}

type HighlightConstructor = new (...ranges: Range[]) => unknown;

interface HighlightGlobals {
  CSS?: { highlights?: HighlightRegistry };
  Highlight?: HighlightConstructor;
}

class TextHighlightService {
  /** Mark `ranges` under `name`. Replaces whatever `name` held before. */
  paint(name: string, ranges: readonly Range[]): void {
    const api = this.#api();
    if (!api) return;
    api.highlights.set(name, new api.Highlight(...ranges));
  }

  clear(name: string): void {
    this.#api()?.highlights.delete(name);
  }

  #api(): { highlights: HighlightRegistry; Highlight: HighlightConstructor } | null {
    try {
      const { CSS, Highlight } = globalThis as HighlightGlobals;
      const highlights = CSS?.highlights;
      return highlights && Highlight ? { highlights, Highlight } : null;
    } catch {
      return null;
    }
  }
}

export const textHighlightService = new TextHighlightService();
