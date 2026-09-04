export function BrowserEmptyState() {
  return (
    <div class="h-full w-full bg-canvas grid place-items-center px-6 text-center text-sm text-ink-300">
      <div class="max-w-sm space-y-2">
        <div class="text-ink-200 font-medium">No running apps</div>
        <div class="text-[12.5px] leading-relaxed">
          Tell the agent to start a dev server (it&apos;ll bind to{" "}
          <code class="font-mono text-ink-100">0.0.0.0</code> on any port).
          Click the refresh icon and pick it from the dropdown.
        </div>
      </div>
    </div>
  );
}
