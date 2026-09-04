import type { SelfUpdateStatus } from "../../models/selfUpdate";
import { Download, Loader, RotateCcw } from "../primitives/icons";
import { UpdateRunCard } from "./updates/UpdateRunCard";
import { formatUpdateTime } from "./updates/updateTime";

export function UpdatesSettings({
  status,
  loading,
  checking,
  applying,
  retrying,
  restarting,
  error,
  onCheck,
  onApply,
  onRetry,
}: {
  status: SelfUpdateStatus | null;
  loading: boolean;
  checking: boolean;
  applying: boolean;
  retrying: boolean;
  restarting: boolean;
  error: string | null;
  onCheck: () => Promise<void>;
  onApply: (tag?: string) => Promise<void>;
  onRetry: () => Promise<void>;
}) {
  if (loading && status == null) {
    return (
      <div class="rounded-card border border-line bg-surface px-4 py-12 flex items-center justify-center gap-2 text-[13px] text-ink-300">
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
  const startingUpdate = applying || retrying;

  return (
    <div class="space-y-4">
      <section class="rounded-card border border-line bg-surface overflow-hidden">
        <header class="px-4 py-3 flex items-start gap-3">
          <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint">
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
                  {` · checked ${formatUpdateTime(lastCheck.checkedAt)}`}
                </>
              )}
            </div>
          </div>
          <button
            type="button"
            onClick={() => void onCheck()}
            disabled={checking || runActive}
            class="h-9 px-2.5 rounded-md inline-flex items-center gap-2 text-[12px] text-ink-200 hover:text-ink-50 hover:bg-tint-strong disabled:opacity-60"
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
            disabled={startingUpdate}
            class="btn btn-primary mt-3 disabled:opacity-50"
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
        <UpdateRunCard
          run={run}
          restarting={restarting}
          retrying={retrying}
          retryDisabled={startingUpdate}
          onRetry={onRetry}
        />
      )}
    </div>
  );
}
