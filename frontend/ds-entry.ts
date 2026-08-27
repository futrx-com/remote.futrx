// Design-sync entry: the curated set of presentational components exported to
// the claude.ai/design bundle. Outside tsconfig's include on purpose — only
// the design-sync converter bundles this file. Keep exports limited to
// components that render standalone (no state/transport imports).
export { DsSurface } from "./ds-surface";
export { LoadingScreen } from "./src/ui/primitives/LoadingScreen";
export {
  Loading,
  Empty,
  Panel,
  Grid,
  Field,
} from "./src/ui/projects/project-containers/ProjectContainerPrimitives";

export { UserMessage } from "./src/ui/chat/messages/UserMessage";
export { MessageBlock } from "./src/ui/chat/messages/MessageBlock";
export { StreamingText } from "./src/ui/chat/messages/StreamingText";
export { ErrorMessage } from "./src/ui/chat/messages/ErrorMessage";
export { ThinkingIndicator } from "./src/ui/chat/messages/ThinkingIndicator";
export { ThreadEmptyState } from "./src/ui/chat/messages/ThreadEmptyState";
export { JumpToLatestButton } from "./src/ui/chat/messages/JumpToLatestButton";

export { ToolShell } from "./src/ui/chat/tool-calls/ToolShell";
export { CodeBlock } from "./src/ui/chat/tool-calls/CodeBlock";
export { DiffView } from "./src/ui/chat/history/DiffView";
export { AskUserQuestion } from "./src/ui/chat/tool-calls/ask-user-question/AskUserQuestion";
export { QuestionOption } from "./src/ui/chat/tool-calls/ask-user-question/QuestionOption";
export { QuestionProgress } from "./src/ui/chat/tool-calls/ask-user-question/QuestionProgress";
export { AnsweredSummary } from "./src/ui/chat/tool-calls/ask-user-question/AnsweredSummary";

export { AttachmentChip } from "./src/ui/chat/composer/AttachmentChip";
export { AttachButton } from "./src/ui/chat/composer/AttachButton";
export { SendControls } from "./src/ui/chat/composer/SendControls";
export { SelectedSkillChips } from "./src/ui/chat/composer/SelectedSkillChips";
export { ComposerDropOverlay } from "./src/ui/chat/composer/ComposerDropOverlay";

export { UsagePill } from "./src/ui/chat/header/UsagePill";
export { ModelPicker } from "./src/ui/chat/header/ModelPicker";
export { WorkspaceActions } from "./src/ui/chat/header/WorkspaceActions";

export { ProjectStatusDot } from "./src/ui/sidebar/ProjectStatusDot";
export { ProjectGroup } from "./src/ui/sidebar/ProjectGroup";
export { AccountFooter } from "./src/ui/sidebar/AccountFooter";
export { WorkspaceSearch } from "./src/ui/sidebar/WorkspaceSearch";
export { SidebarEmptyState, SidebarNoMatches } from "./src/ui/sidebar/SidebarEmptyState";

export { NoChatSelected } from "./src/ui/layout/NoChatSelected";
export { BrowserEmptyState } from "./src/ui/chat/browser/BrowserEmptyState";
export { Markdown } from "./src/ui/chat/markdown/Markdown";
