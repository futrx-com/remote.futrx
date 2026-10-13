import { useState } from "preact/hooks";
import {
  permissionBranches,
  type PermissionBranch,
  type PermissionEntry,
  type RuleState,
} from "../../../state/hooks/permissions/permissionBranches";
import { ChevronLeft, Lock, Search } from "../../primitives/icons";

const STATE_OPTIONS: { state: RuleState; label: string; active: string }[] = [
  { state: "inherit", label: "Inherit", active: "bg-tint-strong text-ink-50" },
  { state: "allow", label: "Allow", active: "bg-accent-green/20 text-accent-green" },
  { state: "deny", label: "Deny", active: "bg-accent-red/20 text-accent-red" },
];

function matches(entry: PermissionEntry, query: string): boolean {
  const needle = query.trim().toLowerCase();
  return !needle || `${entry.definition.key} ${entry.definition.description}`.toLowerCase().includes(needle);
}

function StateSwitch({
  entry,
  disabled,
  onChange,
}: {
  entry: PermissionEntry;
  disabled: boolean;
  onChange: (state: RuleState) => void;
}) {
  const locked = disabled || !entry.editable;
  return (
    <div
      role="radiogroup"
      aria-label={`${entry.definition.key} access`}
      class="inline-flex flex-none rounded-lg border border-line bg-tint p-0.5"
    >
      {STATE_OPTIONS.map((option) => (
        <button
          key={option.state}
          type="button"
          role="radio"
          aria-checked={entry.state === option.state}
          disabled={locked}
          onClick={() => onChange(option.state)}
          class={`h-6 rounded-md px-2 text-[11.5px] font-medium transition-colors disabled:cursor-not-allowed ${
            entry.state === option.state ? option.active : "text-ink-300 hover:text-ink-100 disabled:hover:text-ink-300"
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function EntryRow({
  entry,
  disabled,
  onChange,
}: {
  entry: PermissionEntry;
  disabled: boolean;
  onChange: (state: RuleState) => void;
}) {
  const edge =
    entry.state === "allow" ? "border-l-accent-green" : entry.state === "deny" ? "border-l-accent-red" : "border-l-transparent";
  return (
    <div class={`flex items-center gap-3 rounded-md border border-line border-l-2 bg-tint px-3 py-2 ${edge}`}>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5">
          <span class="truncate font-mono text-[12.5px] text-ink-50" title={entry.definition.key}>
            {entry.action}
          </span>
          {entry.customized && (
            <span class="h-1.5 w-1.5 flex-none rounded-full bg-accent-blue" title="Changed from saved" />
          )}
          {!entry.editable && <Lock class="h-3 w-3 flex-none text-ink-400" aria-label="Locked" />}
        </div>
        <div class="truncate text-[11.5px] text-ink-300" title={entry.definition.description}>
          {entry.state === "inherit"
            ? `Inherited — ${permissionBranches.baselineLabel(entry.definition.baseline)}`
            : entry.definition.description}
        </div>
      </div>
      <StateSwitch entry={entry} disabled={disabled} onChange={onChange} />
    </div>
  );
}

export function BranchConfigurator({
  branch,
  disabled,
  onBack,
  onSetEntry,
  onSetBranch,
}: {
  branch: PermissionBranch;
  disabled: boolean;
  onBack: () => void;
  onSetEntry: (key: string, state: RuleState) => void;
  onSetBranch: (state: RuleState) => void;
}) {
  const [query, setQuery] = useState("");
  const categories = branch.categories
    .map((category) => ({ ...category, entries: category.entries.filter((entry) => matches(entry, query)) }))
    .filter((category) => category.entries.length > 0);

  return (
    <section class="branch-card-flip flex flex-col gap-3 rounded-card border border-accent-blue/40 bg-surface p-4">
      <header class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          class="inline-flex h-7 items-center gap-1 rounded-md px-1.5 text-[12px] text-ink-300 transition-colors hover:bg-tint-strong hover:text-ink-100"
        >
          <ChevronLeft class="h-3.5 w-3.5" /> All branches
        </button>
        <h3 class="text-[14.5px] font-semibold capitalize text-ink-50">{branch.id}</h3>
        <span class="text-[12px] text-ink-300">
          {branch.allowed} allowed · {branch.denied} denied · {branch.total} total
        </span>
        <div class="ml-auto flex items-center gap-1">
          <button
            type="button"
            disabled={disabled}
            onClick={() => onSetBranch("allow")}
            class="h-7 rounded-md px-2 text-[11.5px] text-ink-300 hover:bg-tint-strong hover:text-accent-green disabled:opacity-50"
          >
            Allow all
          </button>
          <button
            type="button"
            disabled={disabled}
            onClick={() => onSetBranch("inherit")}
            class="h-7 rounded-md px-2 text-[11.5px] text-ink-300 hover:bg-tint-strong hover:text-ink-100 disabled:opacity-50"
          >
            Reset all
          </button>
        </div>
      </header>
      <label class="flex items-center gap-2 rounded-lg border border-line bg-tint px-2.5">
        <Search class="h-3.5 w-3.5 flex-none text-ink-400" />
        <input
          value={query}
          onInput={(e) => setQuery((e.target as HTMLInputElement).value)}
          placeholder={`Filter ${branch.id} permissions`}
          aria-label="Filter permissions"
          autocomplete="off"
          spellcheck={false}
          class="h-8 w-full bg-transparent text-[12.5px] text-ink-100 outline-none placeholder:text-ink-400"
        />
      </label>
      <div class="flex max-h-[46vh] flex-col gap-3 overflow-y-auto pr-1">
        {categories.length === 0 && <div class="py-4 text-center text-[12.5px] text-ink-300">No permissions match.</div>}
        {categories.map((category) => (
          <div key={category.name} class="flex flex-col gap-1.5">
            <div class="text-[11px] uppercase tracking-[0.08em] text-ink-300">{category.name}</div>
            {category.entries.map((entry) => (
              <EntryRow
                key={entry.definition.key}
                entry={entry}
                disabled={disabled}
                onChange={(state) => onSetEntry(entry.definition.key, state)}
              />
            ))}
          </div>
        ))}
      </div>
    </section>
  );
}
