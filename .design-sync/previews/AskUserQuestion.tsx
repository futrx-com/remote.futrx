// AskUserQuestion — paginated wizard the agent uses to ask the user questions.
import { AskUserQuestion } from "remote.futrx-web";

const noop = () => {};

const multiQuestionInput = {
  questions: [
    {
      question: "How should we roll out the new push-notification service worker?",
      header: "rollout",
      options: [
        {
          label: "Behind a feature flag",
          description: "Ship dark, enable per-project from the admin panel.",
        },
        {
          label: "Staged by cohort",
          description: "10% of workspaces first, widen daily if error budget holds.",
        },
        {
          label: "Everyone at once",
          description: "Single release; fastest feedback, highest blast radius.",
        },
      ],
    },
    {
      question: "Which browsers must the first release support?",
      header: "support",
      multiSelect: true,
      options: [
        { label: "Chrome / Edge", description: "Full Push API support." },
        { label: "Safari 17+", description: "Requires installed PWA on iOS." },
        { label: "Firefox", description: "Desktop only for now." },
      ],
    },
    {
      question: "Where should delivery failures be reported?",
      header: "alerts",
      options: [
        { label: "Sentry", description: "Existing project, no new setup." },
        { label: "In-app status page", description: "Visible to workspace admins." },
      ],
    },
  ],
};

const singleQuestionInput = {
  questions: [
    {
      question: "The migration will drop the legacy poll_chats table. Proceed?",
      options: [
        { label: "Yes, drop it", description: "All rows were copied to chat_events." },
        { label: "Keep it for one more release", description: "Adds a follow-up task." },
      ],
    },
  ],
};

export const MultiStepWizard = () => (
  <div className="w-full max-w-xl">
    <AskUserQuestion
      toolUseId="preview-askq-multi"
      chatId="preview-chat"
      input={multiQuestionInput}
      onSubmit={noop}
    />
  </div>
);

export const SingleQuestion = () => (
  <div className="w-full max-w-xl">
    <AskUserQuestion
      toolUseId="preview-askq-single"
      chatId="preview-chat"
      input={singleQuestionInput}
      onSubmit={noop}
    />
  </div>
);
