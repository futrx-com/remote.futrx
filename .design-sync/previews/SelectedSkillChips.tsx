// SelectedSkillChips — removable chips for skills attached to the next prompt.
import { SelectedSkillChips } from "remote.futrx-web";

const noop = () => {};

export const Canonical = () => (
  <div className="w-full max-w-xl">
    <SelectedSkillChips
      skills={[
        { name: "code-review", command: "/code-review", provider: "claude", source: "user" },
        { name: "frontend-design", command: "/frontend-design", provider: "claude", source: "plugin" },
        { name: "dataviz", command: "/dataviz", provider: "claude", source: "system" },
      ]}
      onRemove={noop}
    />
  </div>
);

export const SingleSkill = () => (
  <div className="w-full max-w-xl">
    <SelectedSkillChips
      skills={[{ name: "secure-coding", command: "/secure-coding", provider: "claude", source: "user" }]}
      onRemove={noop}
    />
  </div>
);

export const NoSource = () => (
  <div className="w-full max-w-xl">
    <SelectedSkillChips
      skills={[
        { name: "simplify", command: "/simplify" },
        { name: "security-review", command: "/security-review" },
      ]}
      onRemove={noop}
    />
  </div>
);

export const ManyWrapping = () => (
  <div className="w-full max-w-md">
    <SelectedSkillChips
      skills={[
        { name: "code-review", command: "/code-review", source: "user" },
        { name: "frontend-design", command: "/frontend-design", source: "plugin" },
        { name: "dataviz", command: "/dataviz", source: "system" },
        { name: "secure-coding", command: "/secure-coding", source: "user" },
        { name: "sanity-best-practices", command: "/sanity-best-practices", source: "plugin" },
      ]}
      onRemove={noop}
    />
  </div>
);
