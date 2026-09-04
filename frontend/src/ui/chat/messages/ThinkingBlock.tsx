import { useState } from "preact/hooks";
import { ChevronRight, Loader } from "../../primitives/icons";
import { getTextAlignClass, getTextDirection } from "../markdown/bidi";

export function ThinkingBlock({ text, active }: { text: string; active: boolean }) {
  const [open, setOpen] = useState(false);
  const dir = getTextDirection(text);
  const align = getTextAlignClass(text);

  return (
    <div class="my-2 overflow-hidden rounded-card border border-line bg-surface text-sm">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        class="flex min-h-10 w-full items-center gap-2 bg-tint px-3 py-2 text-left transition-colors hover:bg-tint-strong"
      >
        <ChevronRight
          class={`h-3.5 w-3.5 flex-none text-ink-300 transition-transform ${open ? "rotate-90" : ""}`}
          aria-hidden="true"
        />
        <span class="flex-1 text-[13px] font-medium text-ink-300">Thinking</span>
        {active && (
          <Loader class="h-3.5 w-3.5 flex-none animate-spin text-ink-300" aria-hidden="true" />
        )}
      </button>
      {open && (
        <div
          dir={dir}
          style={{ unicodeBidi: "plaintext" }}
          class={`whitespace-pre-wrap border-t border-line bg-inset px-3 py-2.5 text-[13px] leading-[1.65] text-ink-300 [overflow-wrap:anywhere] ${align}`}
          aria-busy={active}
        >
          {text}
        </div>
      )}
    </div>
  );
}
