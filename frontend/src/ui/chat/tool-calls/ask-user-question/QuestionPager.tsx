import { ChevronLeft, ChevronRight } from "../../../primitives/icons";

export function QuestionPager({
  page,
  total,
  canAdvance,
  onPageChange,
  onSubmit,
}: {
  page: number;
  total: number;
  canAdvance: boolean;
  onPageChange: (page: number) => void;
  onSubmit: () => void;
}) {
  const isLast = page === total - 1;

  return (
    <div class="flex items-center justify-between pt-1">
      <button
        type="button"
        onClick={() => onPageChange(Math.max(0, page - 1))}
        disabled={page === 0}
        class="flex items-center gap-1 text-sm px-3 h-10 rounded-md
               text-ink-200 hover:text-ink-100 hover:bg-tint-strong
               disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent"
      >
        <ChevronLeft class="w-3.5 h-3.5" /> Back
      </button>
      {isLast ? (
        <button
          type="button"
          onClick={onSubmit}
          disabled={!canAdvance}
          class="flex items-center gap-1.5 bg-accent-blue hover:bg-accent-blue/85 h-10
                 disabled:bg-ink-500 disabled:cursor-not-allowed
                 text-on-accent text-sm font-medium px-3.5 rounded-control"
        >
          Send answer <ChevronRight class="w-3.5 h-3.5" />
        </button>
      ) : (
        <button
          type="button"
          onClick={() => onPageChange(Math.min(total - 1, page + 1))}
          disabled={!canAdvance}
          class="flex items-center gap-1.5 bg-accent-blue/90 hover:bg-accent-blue h-10
                 disabled:bg-ink-500 disabled:cursor-not-allowed
                 text-on-accent text-sm font-medium px-3.5 rounded-control"
        >
          Next <ChevronRight class="w-3.5 h-3.5" />
        </button>
      )}
    </div>
  );
}
