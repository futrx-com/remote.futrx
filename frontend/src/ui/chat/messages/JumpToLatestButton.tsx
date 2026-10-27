import { ArrowDown } from "../../primitives/icons";

export function JumpToLatestButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      class="absolute bottom-4 right-4 grid h-9 w-9 place-items-center rounded-full border border-line
             bg-raised text-ink-200 shadow-pop transition hover:text-ink-50 active:scale-[0.97]"
      aria-label="Jump to latest message"
      title="Jump to latest"
    >
      <ArrowDown class="w-4 h-4" />
    </button>
  );
}
