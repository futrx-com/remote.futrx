import type { ChatMeta } from "../../models/chat";
import type { WorkspaceSidebarModel } from "../../models/workspace";
import { useProjectDragReorder } from "../../state/hooks/workspace/useProjectDragReorder";
import { ChatRow } from "./ChatRow";
import { ProjectGroup } from "./ProjectGroup";
import { SidebarEmptyState, SidebarNoMatches } from "./SidebarEmptyState";
import { SidebarSkeleton } from "./SidebarSkeleton";
import { WorkspaceSearch } from "./WorkspaceSearch";
import { AccountFooter } from "./AccountFooter";
import { Skeleton } from "../primitives/Skeleton";
import { ChevronLeft, ChevronRight, Plus, Settings, X } from "../primitives/icons";

// Sidebar chrome is deliberately unpainted until you touch it: the list is the
// only thing meant to carry visual weight.
const ghostIconClass =
  "h-8 w-8 flex-none grid place-items-center rounded-control text-ink-300 " +
  "hover:bg-tint-strong hover:text-ink-50 active:scale-[0.97] transition";

export function Sidebar({
  open,
  model,
  loading,
  query,
  collapsed,
  sidebarCollapsed,
  activeChatId,
  account,
  onClose,
  onQueryChange,
  onClearQuery,
  onToggleSidebar,
  onNewProject,
  onNewChatInProject,
  onToggleProject,
  onSelectChat,
  onDeleteChat,
  onToggleChatUnread,
  onForkChat,
  onReorderProjects,
  onOpenProjectContainers,
  onOpenSettings,
  onSignOut,
}: {
  open: boolean;
  model: WorkspaceSidebarModel;
  /** The first workspace snapshot has not landed yet. */
  loading: boolean;
  query: string;
  collapsed: Record<string, boolean>;
  sidebarCollapsed: boolean;
  activeChatId: string | null;
  account?: { email: string; authenticated: boolean };
  onClose: () => void;
  onQueryChange: (query: string) => void;
  onClearQuery: () => void;
  onToggleSidebar: () => void;
  onNewProject: () => void;
  onNewChatInProject: (projectId?: string) => void;
  onToggleProject: (projectId: string) => void;
  onSelectChat: (chatId: string) => void;
  onDeleteChat: (chat: ChatMeta, event: Event) => void;
  onToggleChatUnread: (chat: ChatMeta, event: Event) => void;
  onForkChat: (chat: ChatMeta, event: Event) => void;
  onReorderProjects: (projectIds: string[]) => void;
  onOpenProjectContainers: (projectId: string) => void;
  onOpenSettings?: () => void;
  onSignOut: () => void;
}) {
  const sidebarWidth = sidebarCollapsed ? "md:w-[64px]" : "md:w-[300px]";
  const expandedOnly = sidebarCollapsed ? "md:hidden" : "";
  const canReorderProjects = !model.query && model.visibleProjects.length > 1;
  const drag = useProjectDragReorder({
    projectIds: model.visibleProjects.map((node) => node.project.id),
    enabled: canReorderProjects,
    onReorder: onReorderProjects,
  });

  return (
    <>
      <div
        class={`md:hidden fixed inset-0 z-30 bg-black/50 transition-opacity duration-200
                ${open ? "opacity-100 pointer-events-auto" : "opacity-0 pointer-events-none"}`}
        onClick={onClose}
      />
      <aside
        data-open={open ? "true" : "false"}
        data-collapsed={sidebarCollapsed ? "true" : "false"}
        class={`codex-sidebar codex-window-frame drawer-panel mobile-sheet safe-top fixed md:static z-40 inset-y-0 left-0 w-[min(92vw,380px)] ${sidebarWidth}
                ${open ? "translate-x-0" : "-translate-x-full"} md:translate-x-0
                bg-surface flex flex-col shadow-modal md:shadow-none
                transition-[width,transform] duration-200 ease-out`}
      >
        <header class={`px-2.5 pt-2.5 pb-2 ${sidebarCollapsed ? "md:px-2" : ""}`}>
          <div class={`flex items-center gap-1 min-h-9 ${sidebarCollapsed ? "md:justify-center" : "mb-3"}`}>
            <div class={`flex flex-1 min-w-0 items-center gap-2 pl-1 ${expandedOnly}`}>
              <img
                src="/icon-192.png"
                alt=""
                aria-hidden="true"
                class="h-5 w-5 flex-none rounded object-cover"
              />
              <span class="truncate text-[13px] font-semibold tracking-[-0.01em] text-ink-50">
                Remote workspace
              </span>
            </div>
            <button
              type="button"
              onClick={onNewProject}
              class={`${ghostIconClass} ${expandedOnly}`}
              aria-label="New project"
              title="New project"
            >
              <Plus class="w-4 h-4" />
            </button>
            <button
              type="button"
              onClick={onToggleSidebar}
              class={`hidden md:grid ${ghostIconClass}`}
              aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
              title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
            >
              {sidebarCollapsed ? <ChevronRight class="w-4 h-4" /> : <ChevronLeft class="w-4 h-4" />}
            </button>
            <button
              type="button"
              onClick={onClose}
              class={`md:hidden ${ghostIconClass}`}
              aria-label="Close sidebar"
              title="Close"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <div class={expandedOnly}>
            <WorkspaceSearch query={query} onQueryChange={onQueryChange} onClear={onClearQuery} />
          </div>
        </header>

        {sidebarCollapsed && (
          <div class="hidden md:flex flex-col items-center gap-1 px-2 pb-2">
            <button
              type="button"
              onClick={onNewProject}
              class={ghostIconClass}
              aria-label="New project"
              title="New project"
            >
              <Plus class="w-4 h-4" />
            </button>
            {onOpenSettings && account?.authenticated && (
              <button
                type="button"
                onClick={onOpenSettings}
                class={ghostIconClass}
                aria-label="Settings"
                title="Settings"
              >
                <Settings class="w-4 h-4" />
              </button>
            )}
          </div>
        )}

        <div class={`flex items-baseline justify-between gap-2 px-4 pb-3 pt-3.5 ${expandedOnly}`}>
          <span class="text-[10.5px] font-semibold uppercase tracking-[0.1em] text-ink-400">
            Projects
          </span>
          {loading ? (
            <Skeleton class="h-2 w-7" />
          ) : (
            <span class="text-[11px] tabular-nums text-ink-400">
              {model.totalProjects} · {model.totalChats}
            </span>
          )}
        </div>

        <div class={`flex-1 min-h-0 overflow-y-auto touch-scroll scrollbar-thin px-2 pb-3 space-y-0.5 ${expandedOnly}`}>
          {loading && <SidebarSkeleton />}

          {/* "No projects yet" is a claim about the account, so it waits for
              the snapshot that can actually support it. */}
          {!loading && model.totalProjects === 0 && model.totalChats === 0 && (
            <SidebarEmptyState onNewProject={onNewProject} />
          )}

          {model.query && !model.hasMatches && <SidebarNoMatches />}

          {model.visibleProjects.map((node) => (
            <ProjectGroup
              key={node.project.id}
              project={node.project}
              chats={node.chats}
              visibleChats={node.filteredChats}
              activeChatId={activeChatId}
              collapsed={!model.query && collapsed[node.project.id] === true}
              onToggle={() => onToggleProject(node.project.id)}
              onNewChat={() => onNewChatInProject(node.project.id)}
              onOpenContainer={() => onOpenProjectContainers(node.project.id)}
              onSelectChat={onSelectChat}
              onDeleteChat={onDeleteChat}
              onToggleChatUnread={onToggleChatUnread}
              onForkChat={onForkChat}
              draggable={canReorderProjects}
              dragging={drag.isDragging(node.project.id)}
              dropPosition={drag.dropPositionOf(node.project.id)}
              {...drag.handlersFor(node.project.id)}
            />
          ))}

          {model.visibleLooseChats.length > 0 && (
            <div class="pt-2">
              <div class="px-2 pt-2 pb-1 text-[10.5px] font-semibold uppercase tracking-[0.1em] text-ink-400">
                Unassigned
              </div>
              <div class="space-y-0.5">
                {model.visibleLooseChats.map((chat) => (
                  <ChatRow
                    key={chat.id}
                    chat={chat}
                    active={chat.id === activeChatId}
                    onSelect={() => onSelectChat(chat.id)}
                    onDelete={(event) => onDeleteChat(chat, event)}
                    onToggleUnread={(event) => onToggleChatUnread(chat, event)}
                    onFork={(event) => onForkChat(chat, event)}
                  />
                ))}
              </div>
            </div>
          )}
        </div>

        {account?.authenticated && (
          <div class={expandedOnly}>
            <AccountFooter
              email={account.email}
              onOpenSettings={onOpenSettings}
              onSignOut={onSignOut}
            />
          </div>
        )}
      </aside>
    </>
  );
}
