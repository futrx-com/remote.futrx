// Field — label/value cell for container metadata; mono and warn-tone variants.
import { Field } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full max-w-sm">
    <Field label="Image" value="ubuntu/24.04 (release)" />
  </div>
);

export const Mono = () => (
  <div className="w-full max-w-sm">
    <Field label="Container" value="futrx-web-prod" mono />
  </div>
);

export const WarnTone = () => (
  <div className="w-full max-w-sm">
    <Field label="CLAUDE.md in sync" value="no" mono tone="warn" />
  </div>
);

export const TruncatedValue = () => (
  <div className="w-full max-w-sm">
    <Field
      label="Host source"
      value="/Users/ahmed/dev/very/deeply/nested/workspaces/remote.futrx/frontend"
      mono
    />
  </div>
);
