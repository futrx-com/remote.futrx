import type { SelfUpdateProgress, SelfUpdateRun } from "../../../models/selfUpdate";
import { AlertCircle, Loader, RotateCcw } from "../../primitives/icons";
import { formatUpdateRelativeTime, formatUpdateTime } from "./updateTime";

export function UpdateRunCard({
  run,
  restarting,
  retrying,
  retryDisabled,
  onRetry,
}: {
  run: SelfUpdateRun;
  restarting: boolean;
  retrying: boolean;
  retryDisabled: boolean;
  onRetry: () => Promise<void>;
}) {
  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 border-b border-line">
        {run.state === "running" && (
          <div class="flex items-center gap-2 text-[13.5px] font-semibold text-ink-50">
            <Loader class="w-4 h-4 animate-spin text-accent-blue" />
            {run.updateKind === "application"
              ? `Deploying application release ${run.target}…`
              : `Updating infrastructure to ${run.target}…`}
          </div>
        )}
        {run.state === "succeeded" && (
          <div class="text-[13.5px] font-semibold text-accent-green">
            {run.updateKind === "application" ? "Deployed" : "Updated to"} {run.target}
          </div>
        )}
        {run.state === "failed" && (
          <div class="flex items-center gap-2 text-[13.5px] font-semibold text-accent-red">
            <AlertCircle class="w-4 h-4 flex-none" />
            {run.updateKind === "application" ? "Deployment of" : "Update to"} {run.target} failed
            {typeof run.exitCode === "number" ? ` (exit ${run.exitCode})` : ""}
          </div>
        )}
        <div class="text-[12px] text-ink-300 mt-0.5">
          Started {formatUpdateTime(run.startedAt)}
          {run.startedBy ? ` by ${run.startedBy}` : ""}
          {run.finishedAt ? ` · finished ${formatUpdateTime(run.finishedAt)}` : ""}
        </div>
        {run.state === "running" && restarting && (
          <div class="mt-2 text-[12px] text-accent-yellow">
            {run.updateKind === "application"
              ? "The application is restarting. The update continues in the background and this page will reconnect automatically."
              : "The application is briefly restarting. The update continues in the background and this page will reconnect automatically."}
          </div>
        )}
        {run.state === "running" && run.progress && (
          <UpdateProgress progress={run.progress} />
        )}
        {run.state === "succeeded" && (
          <button
            type="button"
            onClick={() => window.location.reload()}
            class="btn btn-primary btn-sm mt-2.5 font-medium"
          >
            Reload to use the new version
          </button>
        )}
        {run.state === "failed" && (
          <button
            type="button"
            onClick={() => void onRetry()}
            disabled={retryDisabled}
            class="btn btn-primary btn-sm mt-2.5 inline-flex items-center gap-1.5 font-medium disabled:opacity-50"
          >
            <RotateCcw class={`w-3.5 h-3.5 ${retrying ? "animate-spin" : ""}`} />
            {retrying ? "Retrying update…" : "Retry update"}
          </button>
        )}
      </header>
      {run.log && (
        <div class="bg-inset">
          <div class="px-4 pt-2.5 text-[10.5px] text-ink-400">
            Installer log
            {run.logUpdatedAt ? ` · updated ${formatUpdateRelativeTime(run.logUpdatedAt)}` : ""}
          </div>
          <pre class="m-0 px-4 pb-3 pt-1.5 text-[11px] leading-snug font-mono text-ink-200 max-h-72 overflow-auto whitespace-pre-wrap">
            {run.log}
          </pre>
        </div>
      )}
    </section>
  );
}

function UpdateProgress({ progress }: { progress: SelfUpdateProgress }) {
  const completed = Math.min(progress.completed ?? 0, progress.total ?? 0);

  return (
    <div class="mt-3 rounded-md border border-line bg-inset px-3 py-2.5">
      <div class="flex items-center justify-between gap-3 text-[12px]">
        <span class="font-medium text-ink-100">{progress.message}</span>
        {progress.total != null && progress.total > 0 && (
          <span class="flex-none font-mono text-ink-300">
            {completed}/{progress.total}
          </span>
        )}
      </div>
      {progress.currentItem && (
        <div class="mt-1 text-[11.5px] text-ink-300">
          Current workspace: <span class="font-mono text-ink-200">{progress.currentItem}</span>
        </div>
      )}
      {progress.total != null && progress.total > 0 && (
        <div
          class="mt-2 h-1.5 overflow-hidden rounded-full bg-tint-strong"
          role="progressbar"
          aria-label={progress.message}
          aria-valuemin={0}
          aria-valuemax={progress.total}
          aria-valuenow={completed}
        >
          <div
            class="h-full rounded-full bg-accent-blue transition-[width] duration-300"
            style={{
              width: `${Math.min(100, (completed / progress.total) * 100)}%`,
            }}
          />
        </div>
      )}
      <div class="mt-1.5 text-[10.5px] text-ink-400">
        Progress updated {formatUpdateRelativeTime(progress.updatedAt)}
      </div>
    </div>
  );
}
