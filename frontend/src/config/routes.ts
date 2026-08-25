import type { ApplicationPath } from "../types/transport";

function applicationPath(path: ApplicationPath): ApplicationPath {
  return path;
}

export const API_ROUTES = {
  authSession: "/auth/me",
  googleOAuth: "/api/admin/auth/google",
  chats: {
    collection: "/api/chats",
    item: (id: string) => `/api/chats/${encodeURIComponent(id)}`,
    read: (id: string) => `/api/chats/${encodeURIComponent(id)}/read`,
    unread: (id: string) => `/api/chats/${encodeURIComponent(id)}/unread`,
    fork: (id: string) => `/api/chats/${encodeURIComponent(id)}/fork`,
    files: (id: string, path = "") =>
      `/api/chats/${encodeURIComponent(id)}/files${path ? `?path=${encodeURIComponent(path)}` : ""}`,
    filesSearch: (id: string, query: string) =>
      `/api/chats/${encodeURIComponent(id)}/files/search?q=${encodeURIComponent(query)}`,
    fileDownload: (id: string, path: string) =>
      `/api/chats/${encodeURIComponent(id)}/files/download?path=${encodeURIComponent(path)}`,
    folderDownload: (id: string, path = "") =>
      `/api/chats/${encodeURIComponent(id)}/files/download-folder${path ? `?path=${encodeURIComponent(path)}` : ""}`,
    mediaOpen: (id: string, path: string) =>
      `/api/chats/${encodeURIComponent(id)}/media-open?path=${encodeURIComponent(path)}`,
    ideOpen: (id: string, path: string) =>
      `/api/chats/${encodeURIComponent(id)}/ide-open?path=${encodeURIComponent(path)}`,
    events: (id: string, query: string) =>
      `/api/chats/${encodeURIComponent(id)}/events${query ? `?${query}` : ""}`,
    rewind: (id: string) => `/api/chats/${encodeURIComponent(id)}/rewind`,
    historyRepos: (id: string) =>
      `/api/chats/${encodeURIComponent(id)}/history/repos`,
    historyCommits: (id: string, query: string) =>
      `/api/chats/${encodeURIComponent(id)}/history/commits?${query}`,
    historyDiff: (id: string, query: string) =>
      `/api/chats/${encodeURIComponent(id)}/history/diff?${query}`,
    historyCheckout: (id: string) =>
      `/api/chats/${encodeURIComponent(id)}/history/checkout`,
    schedules: (id: string) =>
      `/api/chats/${encodeURIComponent(id)}/schedules`,
  },
  schedules: {
    item: (id: string) => `/api/schedules/${encodeURIComponent(id)}`,
    run: (id: string) => `/api/schedules/${encodeURIComponent(id)}/run`,
  },
  claudeAuth: {
    status: "/api/claude/auth-status",
    startLogin: "/api/claude/login/start",
    submitCode: "/api/claude/login/code",
    cancelLogin: "/api/claude/login/cancel",
  },
  codexAuth: {
    status: "/api/codex/auth-status",
    startDeviceLogin: "/api/codex/login/device",
  },
  kimiAuth: {
    status: "/api/kimi/auth-status",
    startDeviceLogin: "/api/kimi/login/device",
  },
  projects: {
    collection: "/api/projects",
    item: (id: string) => `/api/projects/${encodeURIComponent(id)}`,
    reorder: "/api/projects/reorder",
    start: (id: string) => `/api/projects/${encodeURIComponent(id)}/start`,
    stop: (id: string) => `/api/projects/${encodeURIComponent(id)}/stop`,
    restart: (id: string) => `/api/projects/${encodeURIComponent(id)}/restart`,
    container: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/container`,
    limits: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/limits`,
    repairNetwork: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/repair-network`,
    apps: (id: string) => `/api/projects/${encodeURIComponent(id)}/apps`,
    agentBrowser: (id: string, scope?: string) =>
      `/api/projects/${encodeURIComponent(id)}/agent-browser${scope ? `?scope=${encodeURIComponent(scope)}` : ""}`,
    startAgentBrowser: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/agent-browser/start`,
    secrets: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/secrets`,
    secret: (id: string, key: string) =>
      `/api/projects/${encodeURIComponent(id)}/secrets/${encodeURIComponent(key)}`,
    access: (id: string) => `/api/projects/${encodeURIComponent(id)}/access`,
    accessMember: (id: string, email: string) =>
      `/api/projects/${encodeURIComponent(id)}/access/${encodeURIComponent(email)}`,
    applications: (id: string) =>
      `/api/projects/${encodeURIComponent(id)}/applications`,
    application: (id: string, appId: string) =>
      `/api/projects/${encodeURIComponent(id)}/applications/${encodeURIComponent(appId)}`,
    applicationAction: (id: string, appId: string, action: string) =>
      `/api/projects/${encodeURIComponent(id)}/applications/${encodeURIComponent(appId)}/${action}`,
  },
  applications: {
    catalog: "/api/applications/catalog",
    collection: "/api/applications",
    item: (appId: string) => `/api/applications/${encodeURIComponent(appId)}`,
    action: (appId: string, action: string) =>
      `/api/applications/${encodeURIComponent(appId)}/${action}`,
  },
  settings: "/api/me/settings",
  push: {
    config: "/api/push/config",
    subscriptions: "/api/push/subscriptions",
    subscriptionStatus: "/api/push/subscriptions/status",
    test: "/api/push/test",
    presence: "/api/push/presence",
  },
  serverInfo: "/api/server/info",
  selfUpdate: {
    status: "/api/admin/update/status",
    check: "/api/admin/update/check",
    apply: "/api/admin/update/apply",
  },
  skills: (query: string) => `/api/skills?${query}`,
  uploads: "/api/uploads",
  users: {
    collection: "/api/admin/users",
    item: (email: string) =>
      `/api/admin/users/${encodeURIComponent(email)}`,
    role: (email: string) =>
      `/api/admin/users/${encodeURIComponent(email)}/role`,
  },
} as const;

export const WEB_SOCKET_ROUTES = {
  workspace: applicationPath("/ws/workspace"),
  claudeAuthStatus: applicationPath("/ws/claude/auth-status"),
  codexAuthStatus: applicationPath("/ws/codex/auth-status"),
  kimiAuthStatus: applicationPath("/ws/kimi/auth-status"),
  chat: (chatId: string, sinceSeq: number): ApplicationPath => {
    const route = applicationPath(`/ws/chat/${encodeURIComponent(chatId)}`);
    return sinceSeq > 0
      ? applicationPath(`${route}?since=${sinceSeq}`)
      : route;
  },
  terminal: (chatId: string): ApplicationPath =>
    applicationPath(`/ws/terminal?chat=${encodeURIComponent(chatId)}`),
} as const;
