import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { capitalize } from "../../../config/text.ts";
import type {
  AgentAuthAccountsSnapshot,
  AgentAuthProvider,
} from "../../../models/auth";
import type { ChatProvider } from "../../../models/chat";
import type {
  ComposerModelOption,
  ComposerProviderOption,
} from "../../../models/agentCapabilities";
import { useDismissShortcut } from "../../../state/hooks/shared/useDismissShortcut.ts";
import {
  accountsForProvider,
  resolveProviderAccountId,
} from "../../../services/auth/agentAccountSelectionService.ts";
import {
  Bot,
  Check,
  ChevronDown,
  ChevronLeft,
  Loader,
  Lock,
  RotateCcw,
  Search,
  Users,
  X,
} from "../../primitives/icons";

const PROVIDER_SEARCH_THRESHOLD = 6;
type PickerStep = "providers" | "accounts" | "models";

export function ComposerAgentPicker({
  provider,
  accountId,
  model,
  authProviders,
  streaming,
  providerOptions,
  modelOptions,
  loading,
  refreshing,
  error,
  onChange,
  onRefresh,
}: {
  provider: ChatProvider;
  accountId: string;
  model: string;
  authProviders: readonly AgentAuthProvider[];
  streaming: boolean;
  providerOptions: readonly ComposerProviderOption[];
  modelOptions: readonly ComposerModelOption[];
  loading: boolean;
  refreshing: boolean;
  error: string;
  onChange: (provider: ChatProvider, accountId: string, model: string) => void;
  onRefresh: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [viewProvider, setViewProvider] = useState<ChatProvider>(provider);
  const [viewAccountId, setViewAccountId] = useState("");
  const [mobileStep, setMobileStep] = useState<PickerStep>("providers");
  const rootRef = useRef<HTMLDivElement>(null);

  const selectedProvider = providerOptions.find((option) => option.value === provider);
  const viewedProvider = providerOptions.find((option) => option.value === viewProvider)
    ?? selectedProvider;
  const selectedAccounts = accountsForProvider(authProviders, provider);
  const viewedAccounts = accountsForProvider(authProviders, viewProvider);
  const effectiveAccountId = resolveProviderAccountId(selectedAccounts, accountId);
  const selectedAccount = selectedAccounts?.items.find(
    (account) => account.id === effectiveAccountId,
  );
  const viewedModels = viewedProvider?.value === provider
    ? modelOptions
    : viewedProvider?.models ?? [];
  const providerLabel = selectedProvider?.label || displayProvider(provider);
  const modelLabel = modelOptions.find((option) => option.value === model)?.label || model || "Auto";
  const unavailableReason = selectedProvider?.disabledReason;

  useEffect(() => {
    if (!open) return;
    function closeOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node | null;
      if (target && !rootRef.current?.contains(target)) close();
    }
    window.addEventListener("mousedown", closeOnOutsideClick);
    return () => window.removeEventListener("mousedown", closeOnOutsideClick);
  }, [open]);

  useDismissShortcut(close, { enabled: open });

  useEffect(() => {
    if (loading) close();
  }, [loading]);

  function openPicker() {
    setViewProvider(provider);
    setViewAccountId(effectiveAccountId);
    setMobileStep("providers");
    setQuery("");
    setOpen(true);
  }

  function close() {
    setOpen(false);
    setQuery("");
  }

  function chooseProvider(nextProvider: ChatProvider) {
    const nextAccounts = accountsForProvider(authProviders, nextProvider);
    setViewProvider(nextProvider);
    setViewAccountId(resolveProviderAccountId(
      nextAccounts,
      nextProvider === provider ? accountId : "",
    ));
    // Mirror the desktop Account column: the step is shown even without saved
    // accounts, so a phone never skips from step 1 to step 3.
    setMobileStep("accounts");
    setQuery("");
  }

  function chooseAccount(nextAccountId: string) {
    setViewAccountId(nextAccountId);
    setMobileStep("models");
  }

  function chooseModel(nextModel: string) {
    if (!viewedProvider || viewedProvider.disabled) return;
    const nextAccountId = resolveProviderAccountId(viewedAccounts, viewAccountId);
    close();
    if (
      viewedProvider.value !== provider
      || nextAccountId !== accountId
      || nextModel !== model
    ) {
      onChange(viewedProvider.value, nextAccountId, nextModel);
    }
  }

  function previousMobileStep() {
    setMobileStep(mobileStep === "models" ? "accounts" : "providers");
  }

  const triggerTitle = loading
    ? "Loading available providers, accounts, and models"
    : streaming
      ? "Cannot change provider, account, or model while streaming"
      : unavailableReason || "Choose provider, account, and model";
  const triggerLabel = [providerLabel, selectedAccount?.label, modelLabel]
    .filter(Boolean)
    .join(" · ");

  return (
    <div ref={rootRef} class="codex-agent-picker relative w-[300px] max-w-[38vw] flex-none">
      <button
        type="button"
        onClick={() => open ? close() : openPicker()}
        class={`flex h-7 w-full min-w-0 items-center gap-1.5 rounded-control px-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50
                ${open ? "bg-tint-active text-ink-50" : "text-ink-300 hover:bg-tint-strong hover:text-ink-100"}`}
        disabled={streaming || loading}
        title={triggerTitle}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        {loading ? (
          <Loader class="h-3.5 w-3.5 flex-none animate-spin text-ink-300" />
        ) : unavailableReason ? (
          <Lock class="h-3.5 w-3.5 flex-none text-accent-yellow" />
        ) : (
          <Bot class="h-3.5 w-3.5 flex-none opacity-60" />
        )}
        <span class="sr-only">Provider, account, and model</span>
        <span class="min-w-0 flex-1 truncate text-[11.5px] font-medium">
          {loading ? "Loading agents…" : triggerLabel}
        </span>
        {!loading && <ChevronDown class="h-3 w-3 flex-none opacity-50" />}
      </button>

      {open && !loading && (
        <>
          <button
            type="button"
            class="fixed inset-0 z-40 cursor-default bg-black/55 md:hidden"
            aria-label="Close provider, account, and model picker"
            onClick={close}
          />
          <div
            class="theme-menu-surface fixed inset-x-3 bottom-3 z-50 max-h-[calc(100dvh-1.5rem)] overflow-hidden rounded-panel border border-line bg-raised shadow-modal
                   md:absolute md:inset-x-auto md:bottom-full md:left-0 md:mb-2 md:w-[min(42rem,calc(100vw-1.5rem))] md:rounded-lg"
            role="dialog"
            aria-label="Choose provider, account, and model"
          >
            <div class="flex h-11 items-center justify-between border-b border-line px-3">
              <div class="flex min-w-0 items-center gap-2">
                {mobileStep !== "providers" && (
                  <button
                    type="button"
                    onClick={previousMobileStep}
                    class="-ml-1 rounded p-1 text-ink-300 hover:bg-tint-strong hover:text-ink-100 md:hidden"
                    aria-label={mobileStep === "models" ? "Back to accounts" : "Back to providers"}
                  >
                    <ChevronLeft class="h-4 w-4" />
                  </button>
                )}
                <span class="truncate text-[12px] font-semibold text-ink-100">
                  <span class="md:hidden">{mobileStepTitle(mobileStep, viewedProvider?.label)}</span>
                  <span class="hidden md:inline">Provider → Account → Model</span>
                </span>
              </div>
              <button
                type="button"
                onClick={close}
                class="rounded p-1 text-ink-400 hover:bg-tint-strong hover:text-ink-100 md:hidden"
                aria-label="Close provider, account, and model picker"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <div class="md:hidden">
              {mobileStep === "providers" && (
                <ProviderList
                  options={providerOptions}
                  currentProvider={provider}
                  viewedProvider={viewProvider}
                  query={query}
                  onQueryChange={setQuery}
                  onChoose={chooseProvider}
                />
              )}
              {mobileStep === "accounts" && (
                <AccountList
                  providerLabel={viewedProvider?.label || displayProvider(viewProvider)}
                  accounts={viewedAccounts}
                  currentAccountId={viewProvider === provider ? effectiveAccountId : ""}
                  viewedAccountId={viewAccountId}
                  onChoose={chooseAccount}
                />
              )}
              {mobileStep === "models" && (
                <ModelList
                  provider={provider}
                  model={model}
                  viewedProvider={viewedProvider}
                  options={viewedModels}
                  onChoose={chooseModel}
                />
              )}
            </div>

            <div class="hidden min-h-0 grid-cols-[minmax(0,12rem)_minmax(0,13rem)_minmax(0,1fr)] md:grid">
              <ProviderList
                options={providerOptions}
                currentProvider={provider}
                viewedProvider={viewProvider}
                query={query}
                onQueryChange={setQuery}
                onChoose={chooseProvider}
              />
              <div class="min-w-0 border-l border-line">
                <AccountList
                  providerLabel={viewedProvider?.label || displayProvider(viewProvider)}
                  accounts={viewedAccounts}
                  currentAccountId={viewProvider === provider ? effectiveAccountId : ""}
                  viewedAccountId={viewAccountId}
                  onChoose={chooseAccount}
                />
              </div>
              <div class="min-w-0 border-l border-line">
                <ModelList
                  provider={provider}
                  model={model}
                  viewedProvider={viewedProvider}
                  options={viewedModels}
                  onChoose={chooseModel}
                />
              </div>
            </div>

            <div class="flex min-h-10 items-center justify-between gap-3 border-t border-line px-2 py-1">
              <button
                type="button"
                onClick={() => void onRefresh()}
                disabled={refreshing}
                class="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-[11px] font-medium text-ink-300 transition hover:bg-tint-strong hover:text-ink-100 disabled:cursor-wait disabled:opacity-60"
              >
                {refreshing
                  ? <Loader class="h-3.5 w-3.5 animate-spin" />
                  : <RotateCcw class="h-3.5 w-3.5" />}
                <span>{refreshing ? "Refreshing models…" : "Refresh models"}</span>
              </button>
              {error && (
                <p class="min-w-0 truncate pr-1 text-[11px] text-accent-red" role="status" title={error}>
                  {error}
                </p>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function ProviderList({
  options,
  currentProvider,
  viewedProvider,
  query,
  onQueryChange,
  onChoose,
}: {
  options: readonly ComposerProviderOption[];
  currentProvider: ChatProvider;
  viewedProvider: ChatProvider;
  query: string;
  onQueryChange: (query: string) => void;
  onChoose: (provider: ChatProvider) => void;
}) {
  const filtered = useMemo(() => {
    const term = query.trim().toLowerCase();
    return term
      ? options.filter((option) => option.label.toLowerCase().includes(term))
      : options;
  }, [options, query]);
  const connected = filtered.filter((option) => !option.disabled);
  const unavailable = filtered.filter((option) => option.disabled);

  return (
    <div class="min-h-0">
      <PickerColumnHeader step="1" title="Provider" detail="Choose the agent first" />
      {options.length > PROVIDER_SEARCH_THRESHOLD && (
        <div class="border-b border-line p-2">
          <label class="flex h-8 items-center gap-2 rounded-md border border-line bg-inset px-2">
            <Search class="h-3.5 w-3.5 flex-none text-ink-400" />
            <span class="sr-only">Search providers</span>
            <input
              value={query}
              onInput={(event) => onQueryChange((event.currentTarget as HTMLInputElement).value)}
              class="min-w-0 flex-1 bg-transparent text-[12px] text-ink-100 placeholder:text-ink-500 focus:outline-none"
              placeholder="Search providers"
            />
          </label>
        </div>
      )}
      <div class="max-h-[min(22rem,calc(100dvh-8rem))] overflow-y-auto p-1.5 md:max-h-[18rem]" role="listbox" aria-label="Providers">
        {connected.length > 0 && <ProviderSectionLabel>Connected</ProviderSectionLabel>}
        {connected.map((option) => {
          const current = option.value === currentProvider;
          const viewed = option.value === viewedProvider;
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => onChoose(option.value)}
              class={`flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left transition
                      ${viewed ? "bg-accent-blue/[0.14] text-accent-blue" : "text-ink-100 hover:bg-tint-strong"}`}
              role="option"
              aria-selected={viewed}
            >
              <span class="min-w-0 flex-1 truncate text-[12.5px] font-semibold">{option.label}</span>
              {current && <Check class="h-3.5 w-3.5 flex-none" aria-label="Current provider" />}
            </button>
          );
        })}

        {unavailable.length > 0 && (
          <div class={connected.length > 0 ? "mt-2 border-t border-line pt-2" : ""}>
            <ProviderSectionLabel>Sign in to use</ProviderSectionLabel>
            {unavailable.map((option) => (
              <div
                key={option.value}
                class={`flex gap-2 rounded-md px-2.5 py-2 ${option.value === viewedProvider ? "bg-accent-yellow/[0.08]" : ""}`}
                role="option"
                aria-disabled="true"
                aria-selected={option.value === viewedProvider}
              >
                <Lock class="mt-0.5 h-3.5 w-3.5 flex-none text-accent-yellow" />
                <span class="min-w-0">
                  <span class="block text-[12.5px] font-semibold text-ink-200">{option.label}</span>
                  <span class="mt-0.5 block text-[10.5px] leading-4 text-ink-400">
                    {option.disabledReason || "Log in before selecting this provider."}
                  </span>
                </span>
              </div>
            ))}
          </div>
        )}

        {filtered.length === 0 && (
          <div class="px-2.5 py-4 text-center text-[12px] text-ink-400">No matching providers</div>
        )}
      </div>
    </div>
  );
}

function AccountList({
  providerLabel,
  accounts,
  currentAccountId,
  viewedAccountId,
  onChoose,
}: {
  providerLabel: string;
  accounts?: AgentAuthAccountsSnapshot;
  currentAccountId: string;
  viewedAccountId: string;
  onChoose: (accountId: string) => void;
}) {
  return (
    <div class="min-h-0">
      <PickerColumnHeader step="2" title="Account" detail={`For ${providerLabel}`} />
      <div
        class="max-h-[min(22rem,calc(100dvh-8rem))] overflow-y-auto p-1.5 md:max-h-[18rem]"
        role="listbox"
        aria-label={`${providerLabel} accounts`}
      >
        {accounts?.items.map((account) => {
          const viewed = account.id === viewedAccountId;
          return (
            <button
              key={account.id}
              type="button"
              onClick={() => onChoose(account.id)}
              class={`flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left transition
                      ${viewed ? "bg-accent-blue/[0.14] text-accent-blue" : "text-ink-100 hover:bg-tint-strong"}`}
              role="option"
              aria-selected={viewed}
            >
              <Users class="h-3.5 w-3.5 flex-none opacity-60" aria-hidden="true" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[12.5px] font-semibold">{account.label}</span>
                {account.email && (
                  <span class="mt-0.5 block truncate text-[10.5px] text-ink-400">{account.email}</span>
                )}
              </span>
              {account.id === currentAccountId && (
                <Check class="h-3.5 w-3.5 flex-none" aria-label="Current account" />
              )}
            </button>
          );
        })}
        {!accounts?.items.length && (
          <div class="flex min-h-36 items-center justify-center p-5 text-center">
            <div class="max-w-[13rem]">
              <Users class="mx-auto h-5 w-5 text-ink-500" />
              <div class="mt-2 text-[12.5px] font-semibold text-ink-200">Default provider login</div>
              <p class="mt-1 text-[10.5px] leading-4 text-ink-400">
                {providerLabel} has no saved account choices, so its configured login will be used.
              </p>
              {/* Desktop shows the model column alongside; the phone step needs a way forward. */}
              <button
                type="button"
                onClick={() => onChoose("")}
                class="mt-3 h-9 w-full rounded-md bg-accent-blue/80 px-3 text-[12px] font-medium text-white transition hover:bg-accent-blue md:hidden"
              >
                Use default login
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function ModelList({
  provider,
  model,
  viewedProvider,
  options,
  onChoose,
}: {
  provider: ChatProvider;
  model: string;
  viewedProvider?: ComposerProviderOption;
  options: readonly ComposerModelOption[];
  onChoose: (model: string) => void;
}) {
  if (!viewedProvider) {
    return <div class="px-4 py-8 text-center text-[12px] text-ink-400">Choose a provider</div>;
  }
  if (viewedProvider.disabled) {
    return (
      <div class="flex min-h-44 items-center justify-center p-5 text-center">
        <div class="max-w-[18rem]">
          <Lock class="mx-auto h-5 w-5 text-accent-yellow" />
          <div class="mt-2 text-[13px] font-semibold text-ink-100">Sign in to {viewedProvider.label}</div>
          <p class="mt-1 text-[11px] leading-4 text-ink-400">
            {viewedProvider.disabledReason || "Log in before selecting this provider."}
          </p>
        </div>
      </div>
    );
  }

  const selectedModel = viewedProvider.value === provider ? model : "";
  const hasCustomModel = !!selectedModel && !options.some((option) => option.value === selectedModel);

  return (
    <div class="min-h-0">
      <PickerColumnHeader step="3" title="Model" detail={`For ${viewedProvider.label} account`} />
      <div class="max-h-[min(22rem,calc(100dvh-8rem))] overflow-y-auto p-1.5 md:max-h-[18rem]" role="listbox" aria-label={`${viewedProvider.label} models`}>
        {hasCustomModel && (
          <ModelOption
            value={selectedModel}
            label={selectedModel}
            sub="custom model"
            active
            onChoose={onChoose}
          />
        )}
        {options.map((option) => (
          <ModelOption
            key={option.value || "auto"}
            {...option}
            active={selectedModel === option.value}
            onChoose={onChoose}
          />
        ))}
        {options.length === 0 && !hasCustomModel && (
          <div class="px-3 py-8 text-center text-[12px] text-ink-400">No models reported</div>
        )}
      </div>
    </div>
  );
}

function ModelOption({
  value,
  label,
  sub,
  active,
  onChoose,
}: ComposerModelOption & {
  active: boolean;
  onChoose: (model: string) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChoose(value)}
      class={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left transition
              ${active ? "bg-accent-blue/[0.14] text-accent-blue" : "text-ink-100 hover:bg-tint-strong"}`}
      role="option"
      aria-selected={active}
    >
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[12.5px] font-semibold">{label}</span>
        <span class="mt-0.5 block truncate text-[11px] text-ink-400">{sub}</span>
      </span>
      {active && <Check class="h-3.5 w-3.5 flex-none" />}
    </button>
  );
}

function PickerColumnHeader({ step, title, detail }: { step: string; title: string; detail: string }) {
  return (
    <div class="hidden border-b border-line px-3 py-2 md:block">
      <div class="truncate text-[11px] font-semibold text-ink-200">{step}. {title}</div>
      <div class="mt-0.5 truncate text-[10px] text-ink-500">{detail}</div>
    </div>
  );
}

function ProviderSectionLabel({ children }: { children: string }) {
  return (
    <div class="px-2.5 pb-1 pt-0.5 text-[9.5px] font-semibold uppercase tracking-[0.12em] text-ink-500">
      {children}
    </div>
  );
}

function mobileStepTitle(step: PickerStep, providerLabel?: string): string {
  if (step === "accounts") return `2. Choose ${providerLabel || "provider"} account`;
  if (step === "models") return `3. Choose ${providerLabel || "provider"} model`;
  return "1. Choose provider";
}

function displayProvider(provider: string): string {
  return provider ? capitalize(provider) : "Agent";
}
