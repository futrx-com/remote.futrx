import type { ChatMeta } from "../../models/chat";
import type { WorkspaceSidebarModel } from "../../state/workspace/workspaceSidebarState";
import { ChatRow } from "./ChatRow";
import { ProjectGroup } from "./ProjectGroup";
import { SidebarEmptyState, SidebarNoMatches } from "./SidebarEmptyState";
import { WorkspaceSearch } from "./WorkspaceSearch";
import { AccountFooter } from "./AccountFooter";
import { ChevronLeft, ChevronRight, Plus, Settings, X } from "../primitives/icons";
import { useState } from "preact/hooks";

export function Sidebar({
  open,
  model,
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
  const [dragProjectId, setDragProjectId] = useState<string | null>(null);
  const [dragOverProjectId, setDragOverProjectId] = useState<string | null>(null);
  const canReorderProjects = !model.query && model.visibleProjects.length > 1;

  function reorderProjectList(sourceId: string, targetId: string) {
    if (sourceId === targetId) return;
    const ids = model.visibleProjects.map((node) => node.project.id);
    const sourceIndex = ids.indexOf(sourceId);
    const targetIndex = ids.indexOf(targetId);
    if (sourceIndex < 0 || targetIndex < 0) return;
    const next = ids.slice();
    const [moved] = next.splice(sourceIndex, 1);
    next.splice(targetIndex, 0, moved);
    onReorderProjects(next);
  }

  return (
    <>
      <div
        class={`md:hidden fixed inset-0 z-30 bg-black/60 transition-opacity duration-200
                ${open ? "opacity-100 pointer-events-auto" : "opacity-0 pointer-events-none"}`}
        onClick={onClose}
      />
      <aside
        data-open={open ? "true" : "false"}
        data-collapsed={sidebarCollapsed ? "true" : "false"}
        class={`codex-sidebar codex-window-frame drawer-panel mobile-sheet safe-top fixed md:static z-40 inset-y-0 left-0 w-[min(92vw,380px)] ${sidebarWidth}
                ${open ? "translate-x-0" : "-translate-x-full"} md:translate-x-0
                bg-[#101318] border-r border-white/10 flex flex-col shadow-2xl md:shadow-none
                transition-[width,transform] duration-200 ease-out`}
      >
        <header class={`px-3 pt-3 pb-2 border-b border-white/10 ${sidebarCollapsed ? "md:px-2" : ""}`}>
          <div class={`flex items-center gap-2 min-h-11 ${sidebarCollapsed ? "md:justify-center" : ""}`}>
            <div class={`flex-1 min-w-0 ${expandedOnly}`}>
              <div class="text-[11px] text-ink-300">Workspace</div>
              <div class="text-[15px] font-semibold text-ink-50 truncate">Projects</div>
            </div>
            <button
              type="button"
              onClick={onNewProject}
              class={`h-10 min-w-10 rounded-md bg-accent-blue text-white grid place-items-center hover:bg-accent-blue/90 active:scale-[0.98] transition ${expandedOnly}`}
              aria-label="New project"
              title="New project"
            >
              <Plus class="w-5 h-5" />
            </button>
            <button
              type="button"
              onClick={onToggleSidebar}
              class="hidden md:grid h-10 w-10 rounded-md bg-white/5 text-ink-200 place-items-center hover:bg-white/[0.09] hover:text-ink-50 active:scale-[0.98] transition"
              aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
              title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
            >
              {sidebarCollapsed ? <ChevronRight class="w-5 h-5" /> : <ChevronLeft class="w-5 h-5" />}
            </button>
            <button
              type="button"
              onClick={onClose}
              class="md:hidden h-10 w-10 rounded-md bg-white/5 text-ink-100 grid place-items-center hover:bg-white/10 active:scale-[0.98] transition"
              aria-label="Close sidebar"
              title="Close"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          <div class={expandedOnly}>
            <WorkspaceSearch query={query} onQueryChange={onQueryChange} onClear={onClearQuery} />
          </div>
        </header>

        {sidebarCollapsed && (
          <div class="hidden md:flex flex-col items-center gap-2 px-2 py-3 border-b border-white/10">
            <button
              type="button"
              onClick={onNewProject}
              class="h-10 w-10 rounded-md bg-accent-blue text-white grid place-items-center hover:bg-accent-blue/90 active:scale-[0.98] transition"
              aria-label="New project"
              title="New project"
            >
              <Plus class="w-5 h-5" />
            </button>
            {onOpenSettings && account?.authenticated && (
              <button
                type="button"
                onClick={onOpenSettings}
                class="h-10 w-10 rounded-md bg-white/5 text-ink-300 grid place-items-center hover:bg-white/[0.09] hover:text-ink-50 transition"
                aria-label="Settings"
                title="Settings"
              >
                <Settings class="w-4 h-4" />
              </button>
            )}
          </div>
        )}

        <div class={`px-3 py-2 flex items-center justify-between gap-2 text-[12px] text-ink-300 ${expandedOnly}`}>
          <span>
            {model.totalProjects} project{model.totalProjects === 1 ? "" : "s"}
            {" - "}
            {model.totalChats} chat{model.totalChats === 1 ? "" : "s"}
          </span>
        </div>

        <div class={`flex-1 min-h-0 overflow-y-auto touch-scroll px-2 pb-3 space-y-2 ${expandedOnly}`}>
          {model.totalProjects === 0 && model.totalChats === 0 && (
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
              dragging={dragProjectId === node.project.id}
              dragOver={dragOverProjectId === node.project.id && dragProjectId !== node.project.id}
              onDragStart={(event) => {
                if (!canReorderProjects) return;
                setDragProjectId(node.project.id);
                event.dataTransfer?.setData("text/plain", node.project.id);
                if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
              }}
              onDragOver={(event) => {
                if (!canReorderProjects || !dragProjectId) return;
                event.preventDefault();
                setDragOverProjectId(node.project.id);
              }}
              onDrop={(event) => {
                if (!canReorderProjects) return;
                event.preventDefault();
                const sourceId = dragProjectId || event.dataTransfer?.getData("text/plain") || "";
                reorderProjectList(sourceId, node.project.id);
                setDragProjectId(null);
                setDragOverProjectId(null);
              }}
              onDragEnd={() => {
                setDragProjectId(null);
                setDragOverProjectId(null);
              }}
            />
          ))}

          {model.visibleLooseChats.length > 0 && (
            <div class="pt-2">
              <div class="px-3 pt-2 pb-1 text-[10.5px] uppercase tracking-wider text-ink-400 font-semibold">
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
