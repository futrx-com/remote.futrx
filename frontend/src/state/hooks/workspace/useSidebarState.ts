import { useEffect, useState } from "preact/hooks";
import type { ChatMeta } from "../../../models/chat";
import type { ProjectMeta } from "../../../models/project";
import { sidebarPreferenceService } from "../../../services/workspace/sidebarPreferenceService.ts";

export function useSidebarState(
  open: boolean,
  onClose: () => void,
  projects: ProjectMeta[],
  chats: ChatMeta[]
) {
  const [query, setQuery] = useState("");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() =>
    sidebarPreferenceService.readCollapsedProjects()
  );
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() =>
    sidebarPreferenceService.readCollapsed()
  );

  useEffect(() => {
    if (!open) return;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  useEffect(() => {
    // Nothing to seed before projects load, and pruning against an empty list
    // would drop what the last session remembered.
    if (projects.length === 0) return;
    setCollapsed((current) => {
      const next = sidebarPreferenceService.seedCollapsedProjects(projects, chats, current);
      return sidebarPreferenceService.hasSameCollapsedProjects(current, next) ? current : next;
    });
  }, [projects, chats]);

  useEffect(() => {
    sidebarPreferenceService.writeCollapsedProjects(collapsed);
  }, [collapsed]);

  useEffect(() => {
    sidebarPreferenceService.writeCollapsed(sidebarCollapsed);
  }, [sidebarCollapsed]);

  function toggleCollapsed(id: string) {
    setCollapsed((current) => ({ ...current, [id]: !current[id] }));
  }

  function toggleSidebarCollapsed() {
    setSidebarCollapsed((current) => !current);
  }

  return {
    query,
    setQuery,
    collapsed,
    toggleCollapsed,
    sidebarCollapsed,
    toggleSidebarCollapsed,
  };
}
