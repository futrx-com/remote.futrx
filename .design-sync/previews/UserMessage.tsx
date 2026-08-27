// UserMessage — right-aligned user bubble in the chat thread.
import { UserMessage } from "remote.futrx-web";

export const Short = () => (
  <div className="w-full max-w-xl">
    <UserMessage text="Add dark-mode support to the settings page" t={1} />
  </div>
);

export const WithRewind = () => (
  <div className="w-full max-w-xl">
    <UserMessage
      text="Refactor the sidebar so archived projects collapse into their own group"
      t={2}
      onRewind={() => {}}
    />
  </div>
);

export const Multiline = () => (
  <div className="w-full max-w-xl">
    <UserMessage
      text={`Three things before we ship:
1. Rename the workspace picker
2. Fix the composer focus bug
3. Bump the version to 0.4.0`}
      t={3}
    />
  </div>
);
