import { Loader } from "../../primitives/icons";

export function ThinkingIndicator() {
  return (
    <div class="inline-flex items-center gap-1.5 rounded-full bg-tint px-2.5 py-1 text-[12px] text-ink-400">
      <Loader class="h-3 w-3 animate-spin" />
      Thinking
    </div>
  );
}
