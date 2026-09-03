import type { JSX } from "preact";
import type { FileNode } from "../../../models/files";
import type { WorkspaceFileTreeState } from "../../../state/hooks/chat/useWorkspaceFileBrowser";
import {
  Archive,
  ChevronRight,
  Code,
  Download,
  FileText,
  Film,
  Folder,
  FolderOpen,
  Image,
  Loader,
  Music,
} from "../../primitives/icons";
import { fileService } from "../../../services/files/fileService.ts";
import type { FileCategory } from "../../../models/files.ts";

type IconComponent = (props: JSX.SVGAttributes<SVGSVGElement>) => JSX.Element;

const CATEGORY_META: Record<FileCategory, { Icon: IconComponent; color: string }> = {
  image: { Icon: Image, color: "text-accent-green" },
  video: { Icon: Film, color: "text-accent-purple" },
  audio: { Icon: Music, color: "text-accent-orange" },
  pdf: { Icon: FileText, color: "text-accent-red" },
  archive: { Icon: Archive, color: "text-accent-yellow" },
  code: { Icon: Code, color: "text-accent-blue" },
  data: { Icon: FileText, color: "text-accent-green" },
  text: { Icon: FileText, color: "text-ink-300" },
};

export function FileTreeNodes({
  nodes,
  depth,
  state,
}: {
  nodes: FileNode[];
  depth: number;
  state: WorkspaceFileTreeState;
}) {
  return (
    <ul class={depth > 0 ? "ml-3 border-l border-line pl-1" : ""}>
      {nodes.map((node) =>
        node.isDir ? (
          <FolderRow key={node.path} node={node} depth={depth} state={state} />
        ) : (
          <FileRow
            key={node.path}
            node={node}
            downloadUrl={state.downloadUrl}
            onOpen={state.onOpenFile}
          />
        )
      )}
    </ul>
  );
}

function FolderRow({
  node,
  depth,
  state,
}: {
  node: FileNode;
  depth: number;
  state: WorkspaceFileTreeState;
}) {
  const isOpen = state.expanded.has(node.path);
  const isLoading = state.loading.has(node.path);
  const children = state.childrenByDir.get(node.path);
  const error = state.errorByDir.get(node.path);

  return (
    <li>
      <div
        class="group flex items-center gap-1.5 rounded px-1.5 py-1 hover:bg-tint cursor-pointer select-none"
        role="button"
        tabIndex={0}
        onClick={() => state.onToggle(node.path)}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            state.onToggle(node.path);
          }
        }}
      >
        {isLoading ? (
          <Loader class="w-3.5 h-3.5 flex-none text-ink-400 animate-spin" />
        ) : (
          <ChevronRight
            class={`w-3.5 h-3.5 flex-none text-ink-400 transition-transform ${isOpen ? "rotate-90" : ""}`}
          />
        )}
        {isOpen ? (
          <FolderOpen class="w-4 h-4 flex-none text-accent-blue" />
        ) : (
          <Folder class="w-4 h-4 flex-none text-accent-blue" />
        )}
        <span class="flex-1 min-w-0 truncate text-[13px] text-ink-100">{node.name}</span>
        {children && (
          <span class="text-[11px] text-ink-500 tabular-nums flex-none">{children.length}</span>
        )}
        <a
          href={state.downloadUrl(node)}
          onClick={(event) => event.stopPropagation()}
          class="h-6 w-6 grid place-items-center rounded text-ink-400 hover:text-accent-blue hover:bg-tint-strong
                 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity flex-none"
          title={`Download ${node.name} as zip`}
          aria-label={`Download ${node.name} as zip`}
        >
          <Download class="w-3.5 h-3.5" />
        </a>
      </div>
      {isOpen && error && (
        <div class="ml-3 pl-2 py-1 text-[12px] text-accent-red">{error}</div>
      )}
      {isOpen && children && children.length === 0 && !error && (
        <div class="ml-3 pl-2 py-1 text-[12px] text-ink-500">Empty folder.</div>
      )}
      {isOpen && children && children.length > 0 && (
        <FileTreeNodes nodes={children} depth={depth + 1} state={state} />
      )}
    </li>
  );
}

function FileRow({
  node,
  downloadUrl,
  onOpen,
}: {
  node: FileNode;
  downloadUrl: (node: FileNode) => string;
  onOpen: (node: FileNode) => void;
}) {
  const { Icon, color } = CATEGORY_META[fileService.category(node.name)];
  return (
    <li>
      <div
        class="group flex items-center gap-1.5 rounded px-1.5 py-1 hover:bg-tint cursor-pointer select-none"
        role="button"
        tabIndex={0}
        title={openTitle(node.name)}
        onClick={() => onOpen(node)}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onOpen(node);
          }
        }}
      >
        <span class="w-3.5 flex-none" aria-hidden="true" />
        <Icon class={`w-4 h-4 flex-none ${color}`} />
        <span class="flex-1 min-w-0 truncate text-[13px] text-ink-100">{node.name}</span>
        {node.size != null && (
          <span class="text-[11px] text-ink-500 tabular-nums flex-none">{fileService.formatBytes(node.size)}</span>
        )}
        <a
          href={downloadUrl(node)}
          download={node.name}
          onClick={(event) => event.stopPropagation()}
          class="h-6 w-6 grid place-items-center rounded text-ink-400 hover:text-accent-blue hover:bg-tint-strong
                 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity flex-none"
          title={`Download ${node.name}`}
          aria-label={`Download ${node.name}`}
        >
          <Download class="w-3.5 h-3.5" />
        </a>
      </div>
    </li>
  );
}

/** Flat row used to render server-side search results, showing the full path. */
export function SearchResultRow({
  node,
  downloadUrl,
  onOpen,
}: {
  node: FileNode;
  downloadUrl: (node: FileNode) => string;
  onOpen: (node: FileNode) => void;
}) {
  const dir = fileService.parentDir(node.path);
  const { Icon, color } = node.isDir
    ? { Icon: Folder as IconComponent, color: "text-accent-blue" }
    : CATEGORY_META[fileService.category(node.name)];
  const openable = !node.isDir;
  return (
    <li>
      <div
        class={`group flex items-center gap-1.5 rounded px-1.5 py-1 hover:bg-tint ${openable ? "cursor-pointer select-none" : ""}`}
        role={openable ? "button" : undefined}
        tabIndex={openable ? 0 : undefined}
        title={openable ? openTitle(node.name) : undefined}
        onClick={openable ? () => onOpen(node) : undefined}
        onKeyDown={
          openable
            ? (event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  onOpen(node);
                }
              }
            : undefined
        }
      >
        <Icon class={`w-4 h-4 flex-none ${color}`} />
        <div class="flex-1 min-w-0">
          <div class="truncate text-[13px] text-ink-100">{node.name}</div>
          {dir && <div class="truncate text-[11px] text-ink-500 font-mono">{dir}/</div>}
        </div>
        {!node.isDir && node.size != null && (
          <span class="text-[11px] text-ink-500 tabular-nums flex-none">{fileService.formatBytes(node.size)}</span>
        )}
        <a
          href={downloadUrl(node)}
          download={node.isDir ? undefined : node.name}
          onClick={(event) => event.stopPropagation()}
          class="h-6 w-6 grid place-items-center rounded text-ink-400 hover:text-accent-blue hover:bg-tint-strong
                 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity flex-none"
          title={node.isDir ? `Download ${node.name} as zip` : `Download ${node.name}`}
          aria-label={node.isDir ? `Download ${node.name} as zip` : `Download ${node.name}`}
        >
          <Download class="w-3.5 h-3.5" />
        </a>
      </div>
    </li>
  );
}

// Hover hint describing what a click will do for this file.
function openTitle(name: string): string {
  const target = fileService.openAction(name);
  if (target.action === "media") return `View ${name}`;
  if (target.action === "ide") return `Open ${name} in IDE`;
  return `Download ${name}`;
}
