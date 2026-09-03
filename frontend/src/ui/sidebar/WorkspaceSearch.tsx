import { Search, X } from "../primitives/icons";

export function WorkspaceSearch({
  query,
  onQueryChange,
  onClear,
}: {
  query: string;
  onQueryChange: (query: string) => void;
  onClear: () => void;
}) {
  return (
    <label class="mt-2 flex h-8 items-center gap-2 rounded-control bg-tint px-2.5 transition-colors
                  focus-within:bg-inset focus-within:ring-1 focus-within:ring-accent-blue/50">
      <Search class="h-3.5 w-3.5 flex-none text-ink-400" />
      <input
        value={query}
        onInput={(event) => onQueryChange((event.currentTarget as HTMLInputElement).value)}
        placeholder="Search"
        class="min-w-0 flex-1 bg-transparent text-[13px] text-ink-100 placeholder:text-ink-400 focus:outline-none"
        autocomplete="off"
        spellcheck={false}
      />
      {query && (
        <button
          type="button"
          onClick={onClear}
          class="grid h-5 w-5 flex-none place-items-center rounded text-ink-400 hover:bg-tint-strong hover:text-ink-100"
          aria-label="Clear search"
        >
          <X class="h-3 w-3" />
        </button>
      )}
    </label>
  );
}
