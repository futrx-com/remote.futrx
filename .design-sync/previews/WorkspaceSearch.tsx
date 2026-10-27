// WorkspaceSearch — sidebar search input over projects and chats.
import { WorkspaceSearch } from "remote.futrx-web";

const noop = () => {};

export const Empty = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <WorkspaceSearch query="" onQueryChange={noop} onClear={noop} />
  </div>
);

export const WithQuery = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <WorkspaceSearch query="websocket reconnect" onQueryChange={noop} onClear={noop} />
  </div>
);
