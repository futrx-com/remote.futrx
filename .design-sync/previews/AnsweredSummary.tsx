// AnsweredSummary — collapsed card shown after the user answers the agent.
import { AnsweredSummary } from "remote.futrx-web";

export const SingleAnswer = () => (
  <div className="w-full max-w-xl">
    <AnsweredSummary answered="Answer: Yes, drop it" />
  </div>
);

export const MultiAnswer = () => (
  <div className="w-full max-w-xl">
    <AnsweredSummary answered="rollout: Behind a feature flag · support: Chrome / Edge, Safari 17+ · alerts: Sentry" />
  </div>
);
