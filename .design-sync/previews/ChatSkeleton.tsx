// ChatSkeleton — whole main pane in outline: header, thread, composer. Stands
// in for the chat while the workspace resolves which one to open.
import { ChatSkeleton } from "remote.futrx-web";

export const Default = () => (
  <div
    className="w-full max-w-3xl flex flex-col border border-line rounded-lg overflow-hidden"
    style={{ height: 520 }}
  >
    <ChatSkeleton />
  </div>
);

export const WithSidebarToggle = () => (
  <div
    className="w-full max-w-3xl flex flex-col border border-line rounded-lg overflow-hidden"
    style={{ height: 520 }}
  >
    <ChatSkeleton onHamburger={() => {}} />
  </div>
);
