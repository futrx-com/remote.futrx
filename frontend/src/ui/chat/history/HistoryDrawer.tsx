import { useEffect, useMemo, useState } from "preact/hooks";
import { DirtyWorkingTreeError, type GitHistoryCommit, type GitHistoryRepo } from "../../../models/history";
import { chatApi } from "../../../api/chatApi";
import { Check, Clock, Loader, RotateCcw, X } from "../../primitives/icons";
import { DiffView } from "./DiffView";

export function HistoryDrawer({
  chatId,
  open,
  onClose,
}: {
  chatId: string;
  open: boolean;
  onClose: () => void;
}) {
  const [repos, setRepos] = useState<GitHistoryRepo[]>([]);
  const [selectedRepoId, setSelectedRepoId] = useState("");
  const [commits, setCommits] = useState<GitHistoryCommit[]>([]);
  const [selectedSha, setSelectedSha] = useState("");
  const [diff, setDiff] = useState("");
  const [reposLoading, setReposLoading] = useState(false);
  const [commitsLoading, setCommitsLoading] = useState(false);
  const [diffLoading, setDiffLoading] = useState(false);
  const [checkoutLoading, setCheckoutLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [checkoutMessage, setCheckoutMessage] = useState<string | null>(null);
  const [checkpointOpen, setCheckpointOpen] = useState(false);
  const [checkpointMessage, setCheckpointMessage] = useState("");
  const [checkpointFiles, setCheckpointFiles] = useState<string[]>([]);

  const selectedRepo = useMemo(
    () => repos.find((repo) => repo.id === selectedRepoId) ?? null,
    [repos, selectedRepoId]
  );
  const selectedCommit = useMemo(
    () => commits.find((commit) => commit.sha === selectedSha) ?? null,
    [commits, selectedSha]
  );

  useEffect(() => {
    if (!open) return;
    setCheckoutMessage(null);
    void loadRepos(true);
  }, [chatId, open]);

  useEffect(() => {
    if (!open || !selectedRepoId) return;
    void loadCommits(selectedRepoId);
  }, [open, selectedRepoId]);

  async function loadRepos(refreshCommits = false) {
    setReposLoading(true);
    setError(null);
    try {
      const response = await chatApi.fetchHistoryRepos(chatId);
      const nextRepos = response.repos || [];
      const nextRepoId = nextRepos.some((repo) => repo.id === selectedRepoId)
        ? selectedRepoId
        : nextRepos[0]?.id || "";
      setRepos(nextRepos);
      setSelectedRepoId(nextRepoId);
      if (!nextRepoId) {
        setCommits([]);
        setSelectedSha("");
        setDiff("");
      } else if (refreshCommits) {
        await loadCommits(nextRepoId);
      }
    } catch (err) {
      setError(errorMessage(err));
      setRepos([]);
      setCommits([]);
      setSelectedSha("");
      setDiff("");
    } finally {
      setReposLoading(false);
    }
  }

  async function loadCommits(repoId: string) {
    setCommitsLoading(true);
    setError(null);
    try {
      const response = await chatApi.fetchHistoryCommits(chatId, repoId, 100);
      const nextCommits = response.commits || [];
      const nextSha = nextCommits.some((commit) => commit.sha === selectedSha)
        ? selectedSha
        : nextCommits[0]?.sha || "";
      setCommits(nextCommits);
      setSelectedSha(nextSha);
      if (nextSha) {
        await loadDiff(repoId, nextSha);
      } else {
        setDiff("");
      }
    } catch (err) {
      setError(errorMessage(err));
      setCommits([]);
      setSelectedSha("");
      setDiff("");
    } finally {
      setCommitsLoading(false);
    }
  }

  async function loadDiff(repoId: string, sha: string) {
    setDiffLoading(true);
    setError(null);
    try {
      const response = await chatApi.fetchHistoryDiff(chatId, repoId, sha);
      setDiff(response.diff || "");
      if (response.truncated) setCheckoutMessage("Diff truncated at 768 KB.");
    } catch (err) {
      setError(errorMessage(err));
      setDiff("");
    } finally {
      setDiffLoading(false);
    }
  }

  async function selectRepo(repoId: string) {
    setSelectedRepoId(repoId);
    setSelectedSha("");
    setDiff("");
    setCheckoutMessage(null);
  }

  async function selectCommit(commit: GitHistoryCommit) {
    setSelectedSha(commit.sha);
    setCheckoutMessage(null);
    if (selectedRepoId) await loadDiff(selectedRepoId, commit.sha);
  }

  async function switchToCommit() {
    if (!selectedRepo || !selectedCommit || checkoutLoading) return;
    if (selectedRepo.dirty) {
      openCheckpointDialog(selectedRepo, selectedCommit);
      return;
    }
    await checkoutSelectedCommit("");
  }

  function openCheckpointDialog(repo: GitHistoryRepo, commit: GitHistoryCommit, files = repo.dirtyFiles || []) {
    setCheckpointFiles(files);
    setCheckpointMessage(`Checkpoint before switching to ${commit.shortSha}`);
    setError(null);
    setCheckoutMessage(null);
    setCheckpointOpen(true);
  }

  async function saveCheckpointAndSwitch() {
    if (!checkpointMessage.trim()) {
      setError("Checkpoint message is required.");
      return;
    }
    await checkoutSelectedCommit(checkpointMessage);
  }

  async function checkoutSelectedCommit(message: string) {
    if (!selectedRepo || !selectedCommit || checkoutLoading) return;
    setCheckoutLoading(true);
    setError(null);
    setCheckoutMessage(null);
    try {
      const response = await chatApi.historyCheckout(chatId, selectedRepo.id, selectedCommit.sha, message);
      setRepos((items) => items.map((repo) => (repo.id === response.repo.id ? response.repo : repo)));
      setCheckpointOpen(false);
      setCheckpointFiles([]);
      setCheckpointMessage("");
      const checkpoint = response.checkpointSha ? ` Checkpoint ${response.checkpointSha.slice(0, 7)} saved.` : "";
      setCheckoutMessage(`Switched to ${selectedCommit.shortSha}.${checkpoint}`);
      await loadCommits(selectedRepo.id);
    } catch (err) {
      if (err instanceof DirtyWorkingTreeError && selectedRepo && selectedCommit) {
        openCheckpointDialog(selectedRepo, selectedCommit, err.dirtyFiles);
      } else {
        setError(errorMessage(err));
      }
    } finally {
      setCheckoutLoading(false);
    }
  }

  const canSwitch = !!selectedRepo && !!selectedCommit && selectedRepo.currentSha !== selectedCommit.sha && !checkoutLoading;

  return (
    <aside
      id="workspace-history-pane"
      class={`workspace-pane workspace-history-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line shadow-2xl
              transition-[width,opacity] duration-200 ease-out ${open ? "opacity-100" : "opacity-0 border-l-0 pointer-events-none"}`}
      aria-hidden={!open}
      aria-label="History"
    >
      <div class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out ${open ? "translate-x-0" : "translate-x-full"}`}>
        <header class="workspace-pane-header codex-header flex-none bg-surface border-b border-line px-3 md:px-4 pb-2.5 flex items-center gap-2">
          <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
            <Clock class="w-4 h-4 text-accent-blue" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2 min-w-0">
              <h2 class="truncate text-[15px] md:text-base font-semibold text-ink-50">History</h2>
              <span class={`h-2 w-2 rounded-full flex-none ${selectedRepo ? "bg-accent-green" : "bg-ink-400"}`} />
            </div>
            <div class="truncate text-[12px] text-ink-300">
              {selectedRepo ? `${repoLabel(selectedRepo)} · ${shortSha(selectedRepo.currentSha)} · ${selectedRepo.currentRef}` : reposLoading ? "Loading repos..." : "No git repos"}
            </div>
          </div>
          <button
            type="button"
            onClick={() => void loadRepos(true)}
            disabled={reposLoading || checkoutLoading}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center disabled:cursor-wait"
            title="Refresh history"
            aria-label="Refresh history"
          >
            {reposLoading ? <Loader class="w-4 h-4 animate-spin" /> : <RotateCcw class="w-4 h-4" />}
          </button>
          <button
            type="button"
            onClick={onClose}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
            title="Close history"
            aria-label="Close history"
            data-workspace-pane-close
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        <div class="flex flex-1 min-h-0 flex-col md:grid md:grid-cols-[300px_minmax(0,1fr)]">
          <section class="h-[42%] min-h-0 flex-none border-b border-line flex flex-col bg-inset md:h-auto md:border-b-0 md:border-r">
            <div class="flex-none p-3 border-b border-line space-y-2">
              <select
                value={selectedRepoId}
                onChange={(event) => void selectRepo((event.currentTarget as HTMLSelectElement).value)}
                disabled={reposLoading || repos.length === 0}
                class="w-full h-9 bg-inset border border-line rounded-md px-2 text-[13px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
                title="Git repository"
              >
                {repos.length === 0 ? (
                  <option value="">No repositories</option>
                ) : (
                  repos.map((repo) => (
                    <option key={repo.id} value={repo.id} class="bg-surface text-ink-100">
                      {repoLabel(repo)}{repo.dirty ? " *" : ""}
                    </option>
                  ))
                )}
              </select>
              {selectedRepo && (
                <div class="flex items-center gap-2 text-[12px] text-ink-300 min-w-0">
                  <span class="truncate">{selectedRepo.dirty ? "Dirty" : "Clean"}</span>
                  <span class="text-ink-500">/</span>
                  <span class="truncate">{selectedRepo.path}</span>
                </div>
              )}
            </div>

            <div class="flex-1 min-h-0 overflow-y-auto touch-scroll p-2 space-y-1">
              {commitsLoading ? (
                <div class="h-24 grid place-items-center text-ink-300 text-[13px]">Loading commits...</div>
              ) : commits.length === 0 ? (
                <div class="h-24 grid place-items-center text-ink-300 text-[13px]">No commits</div>
              ) : (
                commits.map((commit) => (
                  <button
                    type="button"
                    key={commit.sha}
                    onClick={() => void selectCommit(commit)}
                    class={`w-full text-left rounded-md border px-2.5 py-2 transition-colors
                            ${commit.sha === selectedSha
                              ? "bg-accent-blue/[0.14] border-accent-blue/35 text-ink-50"
                              : "bg-tint hover:bg-tint-strong border-line text-ink-100"}`}
                    title={commit.sha}
                  >
                    <div class="flex items-center gap-2 min-w-0">
                      <span class="font-mono text-[12px] text-accent-blue flex-none">{commit.shortSha}</span>
                      {commit.isHead && <span class="text-[10px] uppercase tracking-wide text-accent-green flex-none">HEAD</span>}
                    </div>
                    <div class="mt-1 line-clamp-2 text-[13px] leading-5">{commit.subject || "(no subject)"}</div>
                    <div class="mt-1 flex items-center gap-1.5 text-[11px] text-ink-400 min-w-0">
                      <span class="truncate">{commit.authorName || "unknown"}</span>
                      <span class="flex-none">/</span>
                      <span class="flex-none">{formatDate(commit.authorDate)}</span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </section>

          <section class="min-h-0 min-w-0 flex flex-col">
            <div class="flex-none border-b border-line px-3 py-2.5 flex items-center gap-2">
              <div class="min-w-0 flex-1">
                <div class="truncate text-[13px] font-medium text-ink-100">
                  {selectedCommit ? selectedCommit.subject || selectedCommit.shortSha : "Select a commit"}
                </div>
                <div class="truncate text-[12px] text-ink-400">
                  {selectedCommit ? `${selectedCommit.shortSha} · ${selectedCommit.authorName || "unknown"} · ${formatDate(selectedCommit.authorDate)}` : ""}
                </div>
              </div>
              <button
                type="button"
                onClick={() => void switchToCommit()}
                disabled={!canSwitch}
                class="h-9 inline-flex items-center gap-2 px-3 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 disabled:opacity-50 disabled:cursor-not-allowed"
                title="Switch to selected commit"
              >
                {checkoutLoading ? <Loader class="w-4 h-4 animate-spin" /> : <Check class="w-4 h-4" />}
                <span class="text-[12.5px] font-medium">Switch</span>
              </button>
            </div>

            {(error || checkoutMessage) && (
              <div class={`flex-none border-b border-line px-3 py-2 text-[12px] ${error ? "text-accent-red bg-red-500/10" : "text-accent-green bg-accent-green/10"}`}>
                {error || checkoutMessage}
              </div>
            )}

            <div class="flex-1 min-h-0 min-w-0 overflow-auto touch-scroll bg-inset">
              {diffLoading ? (
                <div class="h-full grid place-items-center text-ink-300 text-[13px]">Loading diff...</div>
              ) : diff ? (
                <DiffView diff={diff} />
              ) : (
                <div class="h-full grid place-items-center text-ink-300 text-[13px]">No diff</div>
              )}
            </div>
          </section>
        </div>
      </div>
    </aside>
  );
}

function repoLabel(repo: GitHistoryRepo): string {
  return repo.relativePath === "." ? `${repo.name} / root` : repo.relativePath;
}

function shortSha(sha: string): string {
  return sha ? sha.slice(0, 7) : "unknown";
}

function formatDate(seconds: number): string {
  if (!seconds) return "unknown";
  return new Date(seconds * 1000).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : "history request failed";
}
