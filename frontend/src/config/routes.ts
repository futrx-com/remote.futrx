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
    transcript: (id: string, query: string) =>
      `/api/chats/${encodeURIComponent(id)}/transcript${query ? `?${query}` : ""}`,
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
  agentAuth: {
    catalog: "/api/agent-auth",
    startCodeLogin: (provider: string) =>
      `/api/${encodeURIComponent(provider)}/login/start`,
    submitCode: (provider: string) =>
      `/api/${encodeURIComponent(provider)}/login/code`,
    cancelCodeLogin: (provider: string) =>
      `/api/${encodeURIComponent(provider)}/login/cancel`,
    startDeviceLogin: (provider: string) =>
      `/api/${encodeURIComponent(provider)}/login/device`,
    apiKey: (provider: string) =>
      `/api/${encodeURIComponent(provider)}/login/api-key`,
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
    usage: (id: string, query = "") =>
      `/api/projects/${encodeURIComponent(id)}/usage${query ? `?${query}` : ""}`,
    access: (id: string) => `/api/projects/${encodeURIComponent(id)}/access`,
    accessMember: (id: string, email: string) =>
      `/api/projects/${encodeURIComponent(id)}/access/${encodeURIComponent(email)}`,
  },
  settings: "/api/me/settings",
  security: {
    summary: "/api/me/security",
    enroll: "/api/me/security/2fa/enroll",
    confirm: "/api/me/security/2fa/confirm",
    disable: "/api/me/security/2fa/disable",
    regenerateRecoveryCodes: "/api/me/security/2fa/recovery-codes/regenerate",
    preferences: "/api/me/security/preferences",
    ackAlert: "/api/me/security/alerts/ack",
  },
  usage: {
    summary: (query: string) => `/api/usage/summary${query ? `?${query}` : ""}`,
    records: (query: string) => `/api/usage/records${query ? `?${query}` : ""}`,
    prices: "/api/admin/usage/prices",
    rebuild: "/api/admin/usage/rebuild",
  },
  push: {
    config: "/api/push/config",
    subscriptions: "/api/push/subscriptions",
    subscriptionStatus: "/api/push/subscriptions/status",
    test: "/api/push/test",
    presence: "/api/push/presence",
  },
  auth2fa: {
    verify: "/auth/2fa/verify",
    cancel: "/auth/2fa/cancel",
  },
  serverInfo: "/api/server/info",
  selfUpdate: {
    status: "/api/admin/update/status",
    check: "/api/admin/update/check",
    apply: "/api/admin/update/apply",
  },
  skills: (query: string) => `/api/skills?${query}`,
  agentCapabilities: (query: string) =>
    `/api/agent-capabilities${query ? `?${query}` : ""}`,
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
  agentAuthStatus: (provider: string): ApplicationPath =>
    applicationPath(`/ws/agent-auth/${encodeURIComponent(provider)}`),
  chat: (chatId: string, sinceSeq: number): ApplicationPath => {
    const route = applicationPath(`/ws/chat/${encodeURIComponent(chatId)}`);
    return sinceSeq > 0
      ? applicationPath(`${route}?since=${sinceSeq}`)
      : route;
  },
  terminal: (chatId: string): ApplicationPath =>
    applicationPath(`/ws/terminal?chat=${encodeURIComponent(chatId)}`),
} as const;
