// ProjectGroup — collapsible project section in the sidebar with its chat list.
import { ProjectGroup } from "remote.futrx-web";

const noop = () => {};
const now = Date.now();
const min = 60_000;

const project = {
  id: "prj_01",
  name: "remote.futrx",
  slug: "remote-futrx",
  cwd: "/workspace/remote.futrx",
  containerName: "futrx-remote-futrx",
  status: "running",
  createdAt: now - 40 * 24 * 60 * min,
  updatedAt: now - 5 * min,
};

const chats = [
  {
    id: "chat_01",
    title: "Fix push notification payload encryption",
    model: "opus",
    projectId: "prj_01",
    createdAt: now - 3 * 60 * min,
    lastMessageAt: now - 4 * min,
    lastReadAt: now - 2 * min,
    running: true,
  },
  {
    id: "chat_02",
    title: "Sidebar drag-and-drop project reordering",
    model: "sonnet",
    projectId: "prj_01",
    createdAt: now - 26 * 60 * min,
    lastMessageAt: now - 35 * min,
    lastReadAt: now - 10 * min,
  },
  {
    id: "chat_03",
    title: "Investigate flaky websocket reconnect loop",
    model: "sonnet",
    projectId: "prj_01",
    createdAt: now - 2 * 24 * 60 * min,
    lastMessageAt: now - 90 * min,
    lastReadAt: now - 3 * 24 * 60 * min,
  },
];

const groupHandlers = {
  onToggle: noop,
  onNewChat: noop,
  onOpenContainer: noop,
  onSelectChat: noop,
  onDeleteChat: noop,
  onToggleChatUnread: noop,
  onForkChat: noop,
};

export const Expanded = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={project}
      chats={chats}
      visibleChats={chats}
      activeChatId="chat_02"
      collapsed={false}
      {...groupHandlers}
    />
  </div>
);

export const Collapsed = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={project}
      chats={chats}
      visibleChats={chats}
      activeChatId={null}
      collapsed={true}
      {...groupHandlers}
    />
  </div>
);

export const Provisioning = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={{
        ...project,
        id: "prj_02",
        name: "docs-site",
        slug: "docs-site",
        containerName: "futrx-docs-site",
        status: "provisioning",
      }}
      chats={[]}
      visibleChats={[]}
      activeChatId={null}
      collapsed={false}
      {...groupHandlers}
    />
  </div>
);

export const WithError = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={{
        ...project,
        id: "prj_03",
        name: "legacy-importer",
        slug: "legacy-importer",
        containerName: "futrx-legacy-importer",
        status: "error",
        errorMsg: "container failed to start: incus: image alpine/3.19 not found",
      }}
      chats={chats.slice(2)}
      visibleChats={chats.slice(2)}
      activeChatId={null}
      collapsed={false}
      {...groupHandlers}
    />
  </div>
);

export const NoChatsYet = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={{
        ...project,
        id: "prj_04",
        name: "api-gateway",
        slug: "api-gateway",
        containerName: "futrx-api-gateway",
      }}
      chats={[]}
      visibleChats={[]}
      activeChatId={null}
      collapsed={false}
      {...groupHandlers}
    />
  </div>
);

export const DragTarget = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <ProjectGroup
      project={project}
      chats={chats}
      visibleChats={chats}
      activeChatId={null}
      collapsed={true}
      draggable
      dragOver
      {...groupHandlers}
    />
  </div>
);
