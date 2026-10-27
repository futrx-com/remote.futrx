import { useEffect, useRef } from "preact/hooks";
import type { AppInstallRun } from "../../models/application";
import { AlertCircle, Check, Loader } from "../primitives/icons";

export function ApplicationInstallRunCard({ run }: { run: AppInstallRun }) {
  const log = useRef<HTMLPreElement>(null);
  const following = useRef(true);
  useEffect(() => {
    if (log.current && following.current) log.current.scrollTop = log.current.scrollHeight;
  }, [run.log]);

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-3 py-2.5 border-b border-line">
        <div role="status" class="flex items-center gap-2 text-[13px] font-semibold">
          {run.state === "running" && <><Loader class="w-4 h-4 animate-spin text-accent-blue" />Installing…</>}
          {run.state === "succeeded" && <><Check class="w-4 h-4 text-accent-green" /><span class="text-accent-green">Application installed</span></>}
          {run.state === "failed" && <><AlertCircle class="w-4 h-4 text-accent-red" /><span class="text-accent-red">Installation stopped</span></>}
        </div>
        <div class="mt-1.5 text-[12px] text-ink-200">{run.progress.message}</div>
        {run.progress.total != null && run.progress.total > 0 && (
          <div class="mt-1 text-[11.5px] text-ink-300">
            Projects: {run.progress.completed ?? 0}/{run.progress.total}
            {run.progress.currentItem && <span class="block font-mono">Current project: {run.progress.currentItem}</span>}
          </div>
        )}
        {run.state === "failed" && run.error && (
          <div role="alert" class="mt-2 max-h-24 overflow-auto whitespace-pre-wrap break-words text-[11.5px] text-accent-red">
            {run.error}
          </div>
        )}
      </header>
      {run.log && (
        <div class="bg-inset">
          <div class="px-3 pt-2 text-[10.5px] text-ink-400">Installer log</div>
          <pre
            ref={log}
            role="log"
            aria-label="Installer log"
            onScroll={(event) => {
              const node = event.currentTarget;
              following.current = node.scrollHeight - node.scrollTop - node.clientHeight < 8;
            }}
            class="m-0 px-3 pb-3 pt-1.5 text-[11px] leading-snug font-mono text-ink-200 max-h-64 overflow-auto whitespace-pre-wrap break-words"
          >
            {run.log}
          </pre>
        </div>
      )}
    </section>
  );
}
