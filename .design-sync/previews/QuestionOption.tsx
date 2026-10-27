// QuestionOption — selectable answer card (radio for single, checkbox for multi).
import { QuestionOption } from "remote.futrx-web";

const noop = () => {};

export const SingleSelect = () => (
  <div className="w-full max-w-xl grid grid-cols-1 sm:grid-cols-2 gap-2">
    <QuestionOption
      label="Behind a feature flag"
      description="Ship dark, enable per-project from the admin panel."
      active
      multi={false}
      onClick={noop}
    />
    <QuestionOption
      label="Staged by cohort"
      description="10% of workspaces first, widen daily."
      active={false}
      multi={false}
      onClick={noop}
    />
  </div>
);

export const MultiSelect = () => (
  <div className="w-full max-w-xl grid grid-cols-1 sm:grid-cols-2 gap-2">
    <QuestionOption
      label="Chrome / Edge"
      description="Full Push API support."
      active
      multi
      onClick={noop}
    />
    <QuestionOption
      label="Safari 17+"
      description="Requires installed PWA on iOS."
      active
      multi
      onClick={noop}
    />
    <QuestionOption
      label="Firefox"
      description="Desktop only for now."
      active={false}
      multi
      onClick={noop}
    />
  </div>
);

export const LabelOnly = () => (
  <div className="w-full max-w-xl grid grid-cols-1 sm:grid-cols-2 gap-2">
    <QuestionOption label="Yes, drop it" active={false} multi={false} onClick={noop} />
    <QuestionOption label="Keep it for one more release" active={false} multi={false} onClick={noop} />
  </div>
);
