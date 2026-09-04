import { useEffect, useRef, useState } from "preact/hooks";
import { Markdown } from "../markdown/Markdown";
import { getTextAlignClass, getTextDirection } from "../markdown/bidi";

interface Props {
  text: string;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
}

// Typewriter-style renderer: buffers incoming text and reveals it at a steady
// rate so chunky deltas from claude CLI feel like smooth per-token streaming.
//
// • Base rate: 80 chars/sec (~feels native).
// • If the backlog grows past 200 chars, we accelerate proportionally so a
//   2KB chunk doesn't take 25 seconds to animate.
// • When `streaming` flips false, snap to the full text immediately.
// • For history replay (mounted with streaming=false), render instantly.
export function StreamingText({ text, streaming, chatId, cwd }: Props) {
  const [displayed, setDisplayed] = useState<string>(() => (streaming ? "" : text));
  const targetRef = useRef(text);
  targetRef.current = text;

  // When streaming ends, jump to the full text — don't make the user wait.
  useEffect(() => {
    if (!streaming) setDisplayed(text);
  }, [streaming, text]);

  // Animation loop only runs while streaming is true.
  useEffect(() => {
    if (!streaming) return;
    let raf = 0;
    let last = performance.now();
    const tick = (now: number) => {
      const dt = now - last;
      last = now;
      setDisplayed((prev) => {
        const target = targetRef.current;
        if (prev.length >= target.length) return prev;
        const lag = target.length - prev.length;
        // Steady rate plus catch-up: 80 cps base, +3 cps per char of backlog over 200.
        const cps = lag > 200 ? 80 + (lag - 200) * 3 : 80;
        const add = Math.max(1, Math.round((dt / 1000) * cps));
        return target.slice(0, prev.length + add);
      });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [streaming]);

  // Subtle blinking caret while we're still revealing characters.
  const showCaret = streaming && displayed.length < text.length;
  const dir = getTextDirection(displayed);
  const align = getTextAlignClass(displayed);

  return (
    <div class="relative">
      {streaming ? (
        <div
          dir={dir}
          class={`whitespace-pre-wrap [overflow-wrap:anywhere] ${align}`}
          style={{ unicodeBidi: "plaintext" }}
        >
          {displayed}
        </div>
      ) : (
        <Markdown chatId={chatId} cwd={cwd}>{displayed}</Markdown>
      )}
      {showCaret && (
        <span
          class="inline-block w-1.5 h-4 -mb-0.5 mx-0.5 align-middle bg-accent-blue/80 animate-pulse-fast rounded-sm"
          aria-hidden="true"
        />
      )}
    </div>
  );
}
