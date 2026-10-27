import { MessageSquare } from "../../primitives/icons";

export function ThreadEmptyState({ cwd }: { cwd?: string }) {
  return (
    <div class="mx-auto max-w-md px-4 py-16 text-center text-sm text-ink-300">
      <div class="mx-auto mb-4 grid h-11 w-11 place-items-center rounded-card border border-line text-ink-400">
        <MessageSquare class="h-5 w-5" />
      </div>
      <div class="text-[15px] font-semibold tracking-[-0.01em] text-ink-50">Start a conversation</div>
      <div class="mt-2 text-[12.5px] leading-relaxed text-ink-400">
        The selected agent runs with full tool access in{" "}
        <span class="font-mono text-ink-100">{cwd || "~"}</span>.
        Drop, paste, or upload files to reference them.
      </div>
    </div>
  );
}
