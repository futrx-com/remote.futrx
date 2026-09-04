export function BrowserResizeHandle({
  resizing,
  onPointerDown,
}: {
  resizing: boolean;
  onPointerDown: (event: PointerEvent) => void;
}) {
  return (
    <button
      type="button"
      onPointerDown={onPointerDown}
      class={`hidden sm:block absolute inset-y-0 left-0 z-10 w-2 cursor-col-resize touch-none
              ${resizing ? "bg-accent-blue/35" : "bg-transparent hover:bg-accent-blue/25"}`}
      title="Resize browser preview"
      aria-label="Resize browser preview"
    >
      <span class="absolute left-1/2 top-1/2 h-12 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-ink-500" />
    </button>
  );
}
