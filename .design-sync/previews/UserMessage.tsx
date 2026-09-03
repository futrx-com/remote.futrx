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

// A pasted stack trace: namespaced class names and a deploy path give the line
// no break opportunity, so the bubble has to wrap inside its column rather than
// grow to fit them. See issue #62.
export const UnbrokenRun = () => (
  <div className="w-full max-w-xl">
    <UserMessage
      text={
        "[2026-08-28 04:11:02] production.ERROR: " +
        "Symfony\\Component\\HttpFoundation\\RedirectResponse::__construct(): " +
        "Argument #1 ($url) must be of type string, null given, called in " +
        "/var/www/gamerhead/releases/8c7723daf1de1dec7fab13dc5f80f57161fd23c/api/vendor/laravel/framework/src/Illuminate/Routing/Redirector.php on line 210"
      }
      t={4}
    />
  </div>
);
