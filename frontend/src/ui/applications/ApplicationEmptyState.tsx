export function ApplicationEmptyState({ text }: { text: string }) {
  return (
    <div class="rounded-md border border-dashed border-white/10 px-3 py-4 text-center text-[12.5px] text-ink-400">
      {text}
    </div>
  );
}
