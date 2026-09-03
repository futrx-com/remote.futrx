import type { ComponentChildren } from "preact";
import { useState } from "preact/hooks";
import { AlertCircle, ChevronDown, ChevronRight, Loader } from "../../primitives/icons";

export function ToolShell({
  icon,
  label,
  badge,
  status,
  isError,
  defaultOpen,
  children,
}: {
  icon: ComponentChildren;
  label: ComponentChildren;
  badge?: string;
  status: "running" | "done";
  isError?: boolean;
  defaultOpen?: boolean;
  children?: ComponentChildren;
}) {
  const [open, setOpen] = useState(!!defaultOpen);
  return (
    <div class={`codex-tool-shell my-2 overflow-hidden rounded-card border text-sm
                ${isError ? "border-accent-red/30 bg-accent-red/[0.05]" : "border-line bg-surface"}`}>
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        class={`codex-tool-trigger w-full min-h-10 flex items-center gap-2 px-3 py-2 text-left
                ${isError ? "bg-accent-red/10" : "bg-tint hover:bg-tint"}`}
      >
        {children ? (
          open ? <ChevronDown class="w-3.5 h-3.5 text-ink-300 flex-none" /> : <ChevronRight class="w-3.5 h-3.5 text-ink-300 flex-none" />
        ) : (
          <span class="w-3.5 flex-none" />
        )}
        <span class={`flex-none ${isError ? "text-accent-red" : "text-ink-300"}`}>{icon}</span>
        <span class="flex-1 min-w-0 truncate text-[13px] text-ink-200">{label}</span>
        {badge && <span class="flex-none font-mono text-[11px] text-ink-400">{badge}</span>}
        {status === "running" ? (
          <Loader class="w-3.5 h-3.5 text-ink-300 animate-spin flex-none" />
        ) : isError ? (
          <AlertCircle class="w-3.5 h-3.5 text-accent-red flex-none" />
        ) : null}
      </button>
      {open && children && (
        <div class="border-t border-line bg-inset">
          {children}
        </div>
      )}
    </div>
  );
}
