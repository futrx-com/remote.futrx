export function ExtensionDrawerResizeHandle({
  title,
  resizing,
  onPointerDown,
}: {
  title: string;
  resizing: boolean;
  onPointerDown: (event: PointerEvent) => void;
}) {
  return (
    <button
      type="button"
      onPointerDown={onPointerDown}
      class={`group hidden sm:flex absolute inset-y-0 left-0 z-30 w-3 items-center justify-center cursor-col-resize touch-none
              ${resizing ? "bg-accent-blue/30" : "bg-tint hover:bg-accent-blue/25"}`}
      title={`Drag to resize ${title}`}
      aria-label={`Resize ${title}`}
    >
      <span
        class={`h-16 w-1 rounded-full transition-colors
                ${resizing ? "bg-accent-blue" : "bg-ink-500 group-hover:bg-accent-blue/80"}`}
      />
    </button>
  );
}
