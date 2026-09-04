// ThinkingIndicator — small spinner pill shown while the agent is still working.
import { ThinkingIndicator } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full max-w-md">
    <ThinkingIndicator />
  </div>
);

export const AfterAssistantText = () => (
  <div className="w-full max-w-xl space-y-2">
    <div className="text-[15px] leading-7 text-ink-100">
      Tests pass. Now updating the changelog and bumping the version before opening the PR.
    </div>
    <ThinkingIndicator />
  </div>
);
