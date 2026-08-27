// Empty — empty-state card for project sections; compact variant for dense lists.
import { Empty } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full max-w-xl">
    <Empty text="Select a project from the sidebar." />
  </div>
);

export const Compact = () => (
  <div className="w-full max-w-xl">
    <Empty text="No secrets yet." compact />
  </div>
);

export const CompactMembers = () => (
  <div className="w-full max-w-xl">
    <Empty text="No members yet." compact />
  </div>
);
