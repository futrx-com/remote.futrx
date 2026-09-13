import { useEffect, useState } from "preact/hooks";
import { useTerminalSession } from "../../state/hooks/chat/useTerminalSession";
import { ShieldCheck, Terminal as TerminalIcon, X } from "../primitives/icons";

export function HostTerminalSettings() {
  const [open, setOpen] = useState(false);
  const terminal = useTerminalSession({
    enabled: open,
    target: "host",
    title: "host",
  });

  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(terminal.focus);
    return () => cancelAnimationFrame(frame);
  }, [open, terminal.focus]);

  const statusLabel =
    terminal.status === "connected" ? "Connected" :
    terminal.status === "connecting" ? "Connecting" :
    terminal.status === "error" ? "Error" :
    "Closed";

  return (
    <section class="overflow-hidden rounded-card border border-line bg-surface">
      <div class="flex flex-col gap-4 border-b border-line p-4 sm:flex-row sm:items-center">
        <div class="flex min-w-0 flex-1 items-start gap-3">
          <div class="grid size-10 flex-none place-items-center rounded-md border border-line bg-tint">
            <TerminalIcon class="size-5 text-accent-blue" />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-balance text-[14.5px] font-semibold text-ink-50">Global terminal</h2>
              <span class="rounded-full border border-line bg-tint px-2 py-0.5 text-[11px] font-medium text-ink-300">
                Administrators only
              </span>
            </div>
            <p class="mt-1 text-pretty text-[12.5px] leading-relaxed text-ink-300">
              Work directly on the host server. The persistent tmux session stays available when you close this page.
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={() => setOpen((current) => !current)}
          class="inline-flex h-10 flex-none items-center justify-center gap-2 rounded-md border border-line bg-tint px-3 text-[13px] font-medium text-ink-100 hover:bg-tint-strong"
        >
          {open ? <X class="size-4" /> : <TerminalIcon class="size-4" />}
          {open ? "Close terminal" : "Open global terminal"}
        </button>
      </div>

      <div class="flex items-start gap-2 border-b border-accent-yellow/30 bg-accent-yellow/10 px-4 py-3 text-[12.5px] leading-relaxed text-ink-200">
        <ShieldCheck class="mt-0.5 size-4 flex-none text-accent-yellow" />
        <p class="text-pretty">
          Commands run with the Remote service account and can affect every project and the server itself. Review commands before running them.
        </p>
      </div>

      {terminal.error && (
        <div class="mx-4 mt-4 rounded-md border border-accent-red/30 bg-accent-red/10 px-3 py-2 text-sm text-accent-red">
          {terminal.error}
        </div>
      )}

      {open ? (
        <div class="h-[32rem] min-h-96 p-3">
          <div class="mb-2 flex items-center gap-2 text-[12px] text-ink-300">
            <span class={`size-2 rounded-full ${terminal.status === "connected" ? "bg-accent-green" : "bg-ink-400"}`} />
            <span>{statusLabel} — host</span>
          </div>
          <div
            ref={terminal.hostRef}
            class="h-[calc(100%-1.5rem)] w-full overflow-hidden rounded-md border border-line bg-inset p-2"
            aria-label="Global host terminal"
          />
        </div>
      ) : (
        <div class="p-4 text-pretty text-[13px] text-ink-300">
          Open the terminal to start or reconnect to the shared host session.
        </div>
      )}
    </section>
  );
}
