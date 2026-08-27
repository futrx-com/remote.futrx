// ModelPicker — header dropdown for switching the agent model mid-chat.
import { ModelPicker } from "remote.futrx-web";

const claudeOptions = [
  { value: "", label: "Auto", sub: "server default" },
  { value: "fable", label: "Fable", sub: "most capable" },
  { value: "opus", label: "Opus", sub: "deepest reasoning" },
  { value: "sonnet", label: "Sonnet", sub: "balanced" },
  { value: "haiku", label: "Haiku", sub: "fast" },
];

const codexOptions = [
  { value: "", label: "Auto", sub: "codex default" },
  { value: "gpt-5.6-sol", label: "GPT-5.6 Sol", sub: "flagship preview" },
  { value: "gpt-5.5", label: "GPT-5.5", sub: "frontier coding" },
  { value: "gpt-5.4", label: "GPT-5.4", sub: "strong everyday coding" },
  { value: "gpt-5.4-mini", label: "GPT-5.4 Mini", sub: "fast" },
];

const claudeLabel = (model?: string) => {
  if (!model) return "Auto";
  const match = claudeOptions.find((o) => o.value !== "" && model.toLowerCase().includes(o.value));
  return match ? match.label : model;
};

const codexLabel = (model?: string) => {
  if (!model) return "Auto";
  const match = codexOptions.find((o) => o.value === model);
  return match ? match.label : model;
};

const noRef = { current: null } as any;

export const Closed = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <ModelPicker
      modelRef={noRef}
      open={false}
      model="sonnet"
      streaming={false}
      options={claudeOptions}
      displayLabel={claudeLabel}
      onToggle={() => {}}
      onPick={() => {}}
    />
  </div>
);

export const OpenMenu = () => (
  <div className="w-full max-w-xl flex justify-end items-start" style={{ minHeight: 400 }}>
    <ModelPicker
      modelRef={noRef}
      open
      model="opus"
      streaming={false}
      options={claudeOptions}
      displayLabel={claudeLabel}
      onToggle={() => {}}
      onPick={() => {}}
    />
  </div>
);

export const CodexProvider = () => (
  <div className="w-full max-w-xl flex justify-end items-start" style={{ minHeight: 410 }}>
    <ModelPicker
      modelRef={noRef}
      open
      model="gpt-5.5"
      streaming={false}
      options={codexOptions}
      displayLabel={codexLabel}
      onToggle={() => {}}
      onPick={() => {}}
    />
  </div>
);

export const AutoDefault = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <ModelPicker
      modelRef={noRef}
      open={false}
      model=""
      streaming={false}
      options={claudeOptions}
      displayLabel={claudeLabel}
      onToggle={() => {}}
      onPick={() => {}}
    />
  </div>
);

export const StreamingLocked = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <ModelPicker
      modelRef={noRef}
      open={false}
      model="fable"
      streaming
      options={claudeOptions}
      displayLabel={claudeLabel}
      onToggle={() => {}}
      onPick={() => {}}
    />
  </div>
);
