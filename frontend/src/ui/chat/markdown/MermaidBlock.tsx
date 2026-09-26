import { useEffect, useId, useMemo, useRef, useState } from "preact/hooks";
import { AlertTriangle } from "../../primitives/icons";

// A `flowchart` / `sequenceDiagram` / etc. block lifted out of the assistant
// markdown and rendered through the mermaid library. Mermaid is large (~
// 200kB minified for the core + per-diagram chunks), so we load it lazily on
// first render and reuse the same promise across every block.
//
// The renderer uses the dark theme variants to match the surrounding chat UI;
// that includes a slightly larger font that reads right-to-left text better.

type MermaidAPI = {
  initialize: (config: Record<string, unknown>) => void;
  render: (id: string, source: string) => Promise<{ svg: string }>;
};

let mermaidPromise: Promise<MermaidAPI> | null = null;

async function loadMermaid(): Promise<MermaidAPI> {
  if (!mermaidPromise) {
    mermaidPromise = import("mermaid").then((mod) => {
      // The mermaid package exports the default instance on the module
      // namespace; the dynamic-import envelope handles chunk splitting.
      const mermaid = (mod as unknown as { default: MermaidAPI }).default ?? (mod as unknown as MermaidAPI);
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: "dark",
        fontFamily:
          'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans Arabic", sans-serif',
        flowchart: { useMaxWidth: true, htmlLabels: true, curve: "basis" },
        sequence: { useMaxWidth: true, showSequenceNumbers: false },
      });
      return mermaid;
    }).catch((err) => {
      // On import failure, clear the cache so a later render can retry.
      mermaidPromise = null;
      throw err;
    });
  }
  return mermaidPromise;
}

interface RenderOutcome {
  status: "idle" | "loading" | "rendered" | "error";
  svg?: string;
  error?: string;
}

export function MermaidBlock({
  source,
  idHint,
}: {
  source: string;
  idHint?: string;
}) {
  const componentId = useId();
  // Stable across re-renders for the lifetime of this mount: each visible
  // block needs its own element id because mermaid.render mutates the
  // document with that id as a scratch target.
  const elementId = useMemo(() => `mermaid-${componentId}`, [componentId]);
  const ref = useRef<HTMLDivElement>(null);
  const [outcome, setOutcome] = useState<RenderOutcome>({ status: "idle" });
  const [visible, setVisible] = useState(false);

  // Only kick off rendering when the block scrolls into view. Mermaid's
  // runtime cost is high enough that lazy-rendering matters on long
  // transcripts with many diagrams.
  useEffect(() => {
    if (!ref.current || typeof IntersectionObserver === "undefined") {
      setVisible(true);
      return;
    }
    const node = ref.current;
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setVisible(true);
            observer.disconnect();
            return;
          }
        }
      },
      { rootMargin: "200px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    setOutcome({ status: "loading" });
    loadMermaid()
      .then(async (mermaid) => {
        // mermaid.render mutates the document by inserting a temporary
        // element; we render off-screen and only adopt the resulting SVG
        // string so the host stays declarative.
        const { svg } = await mermaid.render(elementId, source);
        if (cancelled) return;
        setOutcome({ status: "rendered", svg });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : String(err);
        setOutcome({ status: "error", error: message });
      });
    return () => {
      cancelled = true;
    };
  }, [source, visible, elementId]);

  if (outcome.status === "rendered" && outcome.svg) {
    return (
      <div
        ref={ref}
        class="my-3 overflow-x-auto touch-scroll rounded-lg border border-line bg-surface p-4 flex justify-center"
        data-testid="mermaid-block"
        aria-label={idHint ? `${idHint} diagram` : "Mermaid diagram"}
      >
        <div
          class="mermaid-svg-host max-w-full [&_svg]:max-w-full"
          // eslint-disable-next-line react/no-danger -- produced by mermaid.render with strict securityLevel
          dangerouslySetInnerHTML={{ __html: outcome.svg }}
        />
      </div>
    );
  }

  if (outcome.status === "error") {
    return <MermaidFallback source={source} error={outcome.error ?? "Unknown error"} />;
  }

  return (
    <div
      ref={ref}
      class="my-3 flex h-32 items-center justify-center rounded-lg border border-line border-dashed bg-tint text-[12.5px] text-ink-300"
      data-testid="mermaid-block-pending"
    >
      {outcome.status === "loading" ? "Rendering diagram…" : "Diagram will render when visible"}
    </div>
  );
}

function MermaidFallback({ source, error }: { source: string; error: string }) {
  // Mermaid render failed — fall back to the source as a code block so the
  // content is still legible.
  return (
    <div class="my-3 rounded-lg border border-accent-red/35 overflow-hidden">
      <div class="flex items-center gap-1.5 bg-accent-red/15 px-3 py-1.5 text-[11.5px] text-accent-red">
        <AlertTriangle class="h-3.5 w-3.5" /> Diagram failed to render ({error}).
      </div>
      <pre dir="ltr" class="md-code overflow-x-auto touch-scroll p-3 text-[12.5px] leading-relaxed font-mono text-left bg-surface/70">
        <code dir="ltr">{source}</code>
      </pre>
    </div>
  );
}
