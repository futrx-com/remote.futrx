// MessageBlock — dispatcher for one chat turn: user bubble, assistant parts, or error.
import { MessageBlock } from "remote.futrx-web";

export const UserTurn = () => (
  <div className="w-full max-w-xl">
    <MessageBlock
      block={{ type: "user", text: "Why is the sidebar losing scroll position when I switch projects?", t: 1 }}
      streaming={false}
    />
  </div>
);

export const AssistantTurn = () => (
  <div className="w-full max-w-xl">
    <MessageBlock
      block={{
        type: "assistant",
        t: 2,
        isComplete: true,
        parts: [
          {
            kind: "text",
            text: "The scroll position resets because `ProjectList` remounts on every route change. Two fixes:\n\n1. Hoist the list above the router outlet so it survives navigation.\n2. Persist `scrollTop` in a ref keyed by project id.\n\nThe first is cleaner — the component tree stays stable and no restore logic is needed.",
          },
        ],
      }}
      streaming={false}
      chatId="chat_9f2c"
      cwd="~/dev/remote.futrx"
    />
  </div>
);

export const AssistantWithTools = () => (
  <div className="w-full max-w-xl">
    <MessageBlock
      block={{
        type: "assistant",
        t: 3,
        isComplete: true,
        parts: [
          { kind: "text", text: "Let me check how the sidebar mounts before changing anything." },
          {
            kind: "tool",
            id: "toolu_01",
            name: "Read",
            input: { file_path: "frontend/src/ui/sidebar/ProjectGroup.tsx" },
            output: "82 lines",
            status: "done",
          },
          {
            kind: "tool",
            id: "toolu_02",
            name: "Bash",
            input: { command: "npm test -- ProjectGroup", description: "Run sidebar unit tests" },
            output: "PASS src/ui/sidebar/ProjectGroup.test.tsx (6 tests)",
            status: "done",
          },
          {
            kind: "text",
            text: "Confirmed: the group remounts because its `key` includes the active chat id. Dropping the chat id from the key fixes the scroll reset.",
          },
        ],
      }}
      streaming={false}
      chatId="chat_9f2c"
      cwd="~/dev/remote.futrx"
    />
  </div>
);

export const StreamingTurn = () => (
  <div className="w-full max-w-xl">
    <MessageBlock
      block={{
        type: "assistant",
        t: 4,
        isComplete: false,
        parts: [
          { kind: "thinking", text: "The failing test points at the reducer, but the trace shows the selector re-running — I should diff the memo inputs first." },
          {
            kind: "tool",
            id: "toolu_03",
            name: "Grep",
            input: { pattern: "useSelector", path: "frontend/src/state" },
            status: "running",
          },
        ],
      }}
      streaming={true}
      chatId="chat_9f2c"
      cwd="~/dev/remote.futrx"
    />
  </div>
);

export const ErrorTurn = () => (
  <div className="w-full max-w-xl">
    <MessageBlock
      block={{ type: "error", message: "Agent process exited unexpectedly (code 137). The container ran out of memory — retry with a smaller context or restart the workspace.", t: 5 }}
      streaming={false}
    />
  </div>
);
