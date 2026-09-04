// ThreadEmptyState — centered placeholder shown before the first message in a chat.
import { ThreadEmptyState } from "remote.futrx-web";

export const WithWorkspace = () => (
  <div className="w-full max-w-xl">
    <ThreadEmptyState cwd="~/dev/remote.futrx" />
  </div>
);

export const NoWorkspace = () => (
  <div className="w-full max-w-xl">
    <ThreadEmptyState />
  </div>
);
