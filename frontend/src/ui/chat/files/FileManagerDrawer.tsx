import type { FileNode } from "../../../models/files";
import {
  useWorkspaceFileBrowser,
  type WorkspaceFileTreeState,
} from "../../../state/hooks/chat/useWorkspaceFileBrowser";
import { Folder, Loader, RotateCcw, Search, X } from "../../primitives/icons";
import { FileTreeNodes, SearchResultRow } from "./FileTree";

export function FileManagerDrawer({
  chatId,
  open,
  onClose,
}: {
  chatId: string;
  open: boolean;
  onClose: () => void;
}) {
  const files = useWorkspaceFileBrowser({ chatId, active: open });

  const subtitle = files.searchResults
    ? `${files.searchResults.length} result${files.searchResults.length === 1 ? "" : "s"}${files.searchTruncated ? "+" : ""}`
    : files.rootLoading
      ? "Loading..."
      : "workspace";

  return (
    <aside
      id="workspace-files-pane"
      class={`workspace-pane workspace-files-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line shadow-2xl
              transition-[width,opacity] duration-200 ease-out ${open ? "opacity-100" : "opacity-0 border-l-0 pointer-events-none"}`}
      aria-hidden={!open}
      aria-label="Files"
    >
      <div
        class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out ${open ? "translate-x-0" : "translate-x-full"}`}
      >
        <header class="workspace-pane-header codex-header flex-none bg-surface border-b border-line px-3 md:px-4 pb-2.5 flex items-center gap-2">
          <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
            <Folder class="w-4 h-4 text-accent-blue" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="truncate text-[15px] md:text-base font-semibold text-ink-50">Files</h2>
            <div class="truncate text-[12px] text-ink-300 font-mono">{subtitle}</div>
          </div>
          <button
            type="button"
            onClick={() => void files.reset()}
            disabled={files.rootLoading}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center disabled:cursor-wait"
            title="Refresh"
            aria-label="Refresh files"
          >
            {files.rootLoading ? <Loader class="w-4 h-4 animate-spin" /> : <RotateCcw class="w-4 h-4" />}
          </button>
          <button
            type="button"
            onClick={onClose}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
            title="Close files"
            aria-label="Close files"
            data-workspace-pane-close
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        <div class="flex-none px-3 md:px-4 py-2 border-b border-line">
          <div class="relative">
            <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-ink-400 pointer-events-none" />
            <input
              type="text"
              value={files.query}
              onInput={(event) => files.setQuery((event.currentTarget as HTMLInputElement).value)}
              placeholder="Search all files..."
              class="h-8 w-full bg-inset border border-line rounded-md pl-8 pr-8 text-[13px] text-ink-100
                     placeholder:text-ink-500 focus:outline-none focus:border-accent-blue/60"
            />
            {(files.query || files.searching) && (
              <button
                type="button"
                onClick={() => files.setQuery("")}
                class="absolute right-2 top-1/2 -translate-y-1/2 h-5 w-5 grid place-items-center rounded text-ink-400 hover:text-ink-100"
                aria-label="Clear search"
              >
                {files.searching ? <Loader class="w-3.5 h-3.5 animate-spin" /> : <X class="w-3.5 h-3.5" />}
              </button>
            )}
          </div>
        </div>

        <div class="flex-1 min-h-0 overflow-y-auto px-2 md:px-3 py-3">
          {files.searchResults !== null ? (
            <SearchView
              results={files.searchResults}
              truncated={files.searchTruncated}
              searching={files.searching}
              error={files.searchError}
              downloadUrl={files.downloadUrl}
              onOpen={files.openFile}
            />
          ) : (
            <BrowseView
              rootEntries={files.rootEntries}
              rootError={files.rootError}
              rootLoading={files.rootLoading}
              anyTruncated={files.anyTruncated}
              treeState={files.treeState}
            />
          )}
        </div>
      </div>
    </aside>
  );
}

function BrowseView({
  rootEntries,
  rootError,
  rootLoading,
  anyTruncated,
  treeState,
}: {
  rootEntries: FileNode[];
  rootError?: string;
  rootLoading: boolean;
  anyTruncated: boolean;
  treeState: WorkspaceFileTreeState;
}) {
  return (
    <>
      {rootError && (
        <div class="mb-3 text-[13px] text-accent-red bg-accent-red/10 border border-accent-red/30 rounded-md px-3 py-2">
          {rootError}
        </div>
      )}
      {anyTruncated && (
        <div class="mb-3 text-[12px] text-accent-yellow bg-accent-yellow/[0.08] border border-accent-yellow/25 rounded-md px-3 py-2">
          A folder was large and its listing was truncated. Use search or download the folder as a zip to get everything.
        </div>
      )}
      {!rootLoading && !rootError && rootEntries.length === 0 ? (
        <div class="text-[13px] text-ink-400 px-2 py-2">This workspace is empty.</div>
      ) : (
        <FileTreeNodes nodes={rootEntries} depth={0} state={treeState} />
      )}
    </>
  );
}

function SearchView({
  results,
  truncated,
  searching,
  error,
  downloadUrl,
  onOpen,
}: {
  results: FileNode[];
  truncated: boolean;
  searching: boolean;
  error: string | null;
  downloadUrl: (node: FileNode) => string;
  onOpen: (node: FileNode) => void;
}) {
  return (
    <>
      {error && (
        <div class="mb-3 text-[13px] text-accent-red bg-accent-red/10 border border-accent-red/30 rounded-md px-3 py-2">
          {error}
        </div>
      )}
      {truncated && (
        <div class="mb-3 text-[12px] text-accent-yellow bg-accent-yellow/[0.08] border border-accent-yellow/25 rounded-md px-3 py-2">
          Showing the first matches only — refine your search to narrow it down.
        </div>
      )}
      {!searching && !error && results.length === 0 ? (
        <div class="text-[13px] text-ink-400 px-2 py-2">No matches.</div>
      ) : (
        <ul>
          {results.map((node) => (
            <SearchResultRow key={node.path} node={node} downloadUrl={downloadUrl} onOpen={onOpen} />
          ))}
        </ul>
      )}
    </>
  );
}
