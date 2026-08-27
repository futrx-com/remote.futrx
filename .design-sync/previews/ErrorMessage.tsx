// ErrorMessage — inline red alert row for failed turns in the chat thread.
import { ErrorMessage } from "remote.futrx-web";

export const SessionError = () => (
  <div className="w-full max-w-xl">
    <ErrorMessage message="Failed to reach the agent: connection to the workspace container timed out after 30s." />
  </div>
);

export const DetailedError = () => (
  <div className="w-full max-w-xl">
    <ErrorMessage message="Agent process exited unexpectedly (code 137). The container ran out of memory while indexing node_modules — retry with a smaller context, or restart the workspace from the sidebar." />
  </div>
);
