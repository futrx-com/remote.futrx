import type { SettingsTab } from "../../models/workspace";

export interface WorkspaceRoute {
  view: "chat" | "settings";
  chatId: string | null;
  tab: SettingsTab;
}

const SETTINGS_TABS = new Set<SettingsTab>([
  "appearance", "notifications", "agents", "users", "security",
  "applications", "updates", "info", "usage",
]);

export function parseWorkspaceRoute(pathname: string, search = ""): WorkspaceRoute {
  const legacyChat = new URLSearchParams(search).get("chat");
  if (legacyChat && /^[0-9a-f]{4,32}$/.test(legacyChat)) {
    return { view: "chat", chatId: legacyChat, tab: "appearance" };
  }
  const chat = /^\/chats\/([0-9a-f]{4,32})\/?$/.exec(pathname);
  if (chat) return { view: "chat", chatId: chat[1], tab: "appearance" };
  const settings = /^\/settings(?:\/([^/]+))?\/?$/.exec(pathname);
  if (settings) {
    const requestedTab = settings[1] as SettingsTab | undefined;
    return {
      view: "settings",
      chatId: null,
      tab: requestedTab && SETTINGS_TABS.has(requestedTab) ? requestedTab : "appearance",
    };
  }
  return { view: "chat", chatId: null, tab: "appearance" };
}

export function workspaceRoutePath(route: WorkspaceRoute): string {
  if (route.view === "settings") {
    return route.tab === "appearance" ? "/settings" : `/settings/${route.tab}`;
  }
  return route.chatId ? `/chats/${encodeURIComponent(route.chatId)}` : "/";
}

export function currentWorkspaceRoute(): WorkspaceRoute {
  const route = parseWorkspaceRoute(window.location.pathname, window.location.search);
  const canonicalPath = workspaceRoutePath(route);
  const routedPath = window.location.pathname.startsWith("/chats/") ||
    window.location.pathname === "/settings" ||
    window.location.pathname.startsWith("/settings/");
  if (new URLSearchParams(window.location.search).has("chat") ||
      (routedPath && window.location.pathname !== canonicalPath)) {
    window.history.replaceState(null, "", canonicalPath);
  }
  return route;
}

export function navigateWorkspace(route: WorkspaceRoute, replace = false): void {
  const path = workspaceRoutePath(route);
  if (window.location.pathname === path && !window.location.search) return;
  window.history[replace ? "replaceState" : "pushState"](null, "", path);
}
