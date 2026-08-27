// NoChatSelected — main-pane placeholder shown before any chat is opened.
import { NoChatSelected } from "remote.futrx-web";

const noop = () => {};

export const WithProjects = () => (
  <div className="w-full max-w-2xl flex flex-col border border-white/10 rounded-lg overflow-hidden" style={{ minHeight: 420 }}>
    <NoChatSelected
      hasProjects
      onNewProject={noop}
      onNewChat={noop}
      onHamburger={noop}
    />
  </div>
);

export const FirstRun = () => (
  <div className="w-full max-w-2xl flex flex-col border border-white/10 rounded-lg overflow-hidden" style={{ minHeight: 420 }}>
    <NoChatSelected
      hasProjects={false}
      onNewProject={noop}
      onNewChat={noop}
      onHamburger={noop}
    />
  </div>
);
