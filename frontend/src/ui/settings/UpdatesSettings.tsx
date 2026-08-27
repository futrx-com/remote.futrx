import type { SelfUpdateStatus } from "../../models/selfUpdate";
import { AlertCircle, Download, Loader, RotateCcw } from "../primitives/icons";

export function UpdatesSettings({
  status,
  loading,
  checking,
  applying,
  restarting,
  error,
  onCheck,
  onApply,
}: {
  status: SelfUpdateStatus | null;
  loading: boolean;
  checking: boolean;
  applying: boolean;
  restarting: boolean;
  error: string | null;
  onCheck: () => Promise<void>;
  onApply: (tag?: string) => Promise<void>;
}) {
  if (loading && status == null) {
    return (
      <div class="rounded-lg border border-white/10 bg-[#101318] px-4 py-12 flex items-center justify-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading update status…
      </div>
    );
  }

  const run = status?.run ?? null;
  const lastCheck = status?.lastCheck ?? null;
  const runActive = run?.state === "running";
  const latestTag = lastCheck?.latestTag ?? "";
  const updateAvailable = !runActive && lastCheck?.updateAvailable === true && latestTag !== "";
  const updateKind = lastCheck?.updateKind ?? "infrastructure";
  const infrastructureUpdate = updateKind === "infrastructure";

  return (
    <div class="space-y-4">
      <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
        <header class="px-4 py-3 flex items-start gap-3">
          <div class="mt-0.5 w-9 h-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
            <Download class="w-4 h-4 text-ink-200" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-[14.5px] font-semibold text-ink-50">Application version</div>
            <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
              Running <span class="font-mono text-ink-100">{status?.currentVersion || "unknown"}</span>
              {lastCheck && !lastCheck.error && (
                <>
                  {" · "}
                  {lastCheck.updateAvailable && latestTag !== ""
                    ? `release ${latestTag} is available`
                    : "up to date with the newest release"}
                  {` · checked ${formatTime(lastCheck.checkedAt)}`}
                </>
              )}
            </div>
          </div>
          <button
            type="button"
            onClick={() => void onCheck()}
            disabled={checking || runActive}
            class="h-9 px-2.5 rounded-md inline-flex items-center gap-2 text-[12px] text-ink-200 hover:text-ink-50 hover:bg-white/[0.08] disabled:opacity-60"
          >
            <RotateCcw class={`w-3.5 h-3.5 ${checking ? "animate-spin" : ""}`} />
            <span class="hidden sm:inline">{checking ? "Checking…" : "Check for updates"}</span>
          </button>
        </header>
        {lastCheck?.error && (
          <div class="border-t border-accent-red/20 bg-accent-red/[0.05] px-4 py-2 text-[12px] text-accent-red">
            The release check failed: {lastCheck.error}
          </div>
        )}
        {error && (
          <div class="border-t border-accent-red/20 bg-accent-red/[0.05] px-4 py-2 text-[12px] text-accent-red">
            {error}
          </div>
        )}
      </section>

      {updateAvailable && (
        <section class="rounded-lg border border-accent-blue/25 bg-accent-blue/[0.06] p-4">
          <div class="text-[13.5px] font-semibold text-ink-50">
            {infrastructureUpdate ? "Infrastructure" : "Application"} release {latestTag} is ready
          </div>
          <p class="mt-1 text-[12.5px] leading-relaxed text-ink-300">
            {infrastructureUpdate ? (
              <>
                This release crosses a major or minor version boundary. It converges the host,
                rebuilds the application and base image, and recycles idle project containers.
                Containers with a running agent are skipped. Expect a few minutes where the app
                is unreachable.
              </>
            ) : (
              <>
                This patch release rebuilds and restarts the frontend/backend application only.
                Host configuration, the base image, and project containers remain unchanged.
                Expect a brief reconnect while the service restarts.
              </>
            )}
          </p>
          <button
            type="button"
            onClick={() => void onApply(latestTag)}
            disabled={applying}
            class="mt-3 h-10 px-3 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50"
          >
            {applying
              ? infrastructureUpdate
                ? "Starting infrastructure update…"
                : "Starting application deploy…"
              : infrastructureUpdate
                ? `Update infrastructure to ${latestTag}`
                : `Deploy ${latestTag}`}
          </button>
        </section>
      )}

      {run && (
        <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
          <header class="px-4 py-3 border-b border-white/[0.06]">
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
              Started {formatTime(run.startedAt)}
              {run.startedBy ? ` by ${run.startedBy}` : ""}
              {run.finishedAt ? ` · finished ${formatTime(run.finishedAt)}` : ""}
            </div>
            {run.state === "running" && restarting && (
              <div class="mt-2 text-[12px] text-accent-yellow">
                {run.updateKind === "application"
                  ? "The application is restarting — reconnecting…"
                  : "The server is restarting as part of the update — reconnecting… This can take a few minutes while containers are recycled."}
              </div>
            )}
            {run.state === "succeeded" && (
              <button
                type="button"
                onClick={() => window.location.reload()}
                class="mt-2.5 h-9 px-3 rounded bg-accent-green/80 hover:bg-accent-green text-white text-[13px] font-medium"
              >
                Reload to use the new version
              </button>
            )}
          </header>
          {run.log && (
            <pre class="m-0 px-4 py-3 text-[11px] leading-snug font-mono text-ink-200 bg-black/30 max-h-72 overflow-auto whitespace-pre-wrap">
              {run.log}
            </pre>
          )}
        </section>
      )}
    </div>
  );
}

function formatTime(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
