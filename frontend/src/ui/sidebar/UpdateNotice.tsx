import { Download } from "../primitives/icons";

export function UpdateNotice({ tag, collapsed, onOpen }: {
  tag: string;
  collapsed: boolean;
  onOpen: () => void;
}) {
  return (
    <div class="mt-auto px-2.5 pb-2">
      <button
        type="button"
        onClick={onOpen}
        title={`Release ${tag} is available — open Updates`}
        aria-label={`Update available: ${tag}. Open Updates`}
        class={`flex w-full items-center gap-2 rounded-control bg-accent-blue/10 px-2.5 py-2 text-accent-blue transition-colors hover:bg-accent-blue/20 ${collapsed ? "md:justify-center md:px-0" : ""}`}
      >
        <Download class="h-4 w-4 flex-none" />
        <span class={`min-w-0 flex-1 text-left text-[12px] font-medium ${collapsed ? "md:hidden" : ""}`}>
          Update available
        </span>
        <span class={`max-w-[7rem] truncate text-[11px] ${collapsed ? "md:hidden" : ""}`}>
          {tag}
        </span>
      </button>
    </div>
  );
}
