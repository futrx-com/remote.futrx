import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import type { ChatProvider } from "../../../models/chat";
import type {
  ComposerModelOption,
  ComposerProviderOption,
} from "../../../models/agentCapabilities";
import {
  Bot,
  Check,
  ChevronDown,
  ChevronLeft,
  Loader,
  Lock,
  RotateCcw,
  Search,
  X,
} from "../../primitives/icons";

const PROVIDER_SEARCH_THRESHOLD = 6;

export function ComposerAgentPicker({
  provider,
  model,
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
  model: string;
  streaming: boolean;
  providerOptions: readonly ComposerProviderOption[];
  modelOptions: readonly ComposerModelOption[];
  loading: boolean;
  refreshing: boolean;
  error: string;
  onChange: (provider: ChatProvider, model: string) => void;
  onRefresh: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [viewProvider, setViewProvider] = useState<ChatProvider>(provider);
  const [mobileStep, setMobileStep] = useState<"providers" | "models">("providers");
  const rootRef = useRef<HTMLDivElement>(null);

  const selectedProvider = providerOptions.find((option) => option.value === provider);
  const viewedProvider = providerOptions.find((option) => option.value === viewProvider)
    ?? selectedProvider;
  const viewedModels = viewedProvider?.value === provider ? modelOptions : viewedProvider?.models ?? [];
  const providerLabel = selectedProvider?.label || displayProvider(provider);
  const modelLabel = modelOptions.find((option) => option.value === model)?.label || model || "Auto";
  const unavailableReason = selectedProvider?.disabledReason;

  useEffect(() => {
    if (!open) return;
    function closeOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node | null;
      if (target && !rootRef.current?.contains(target)) close();
    }
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") close();
    }
    window.addEventListener("mousedown", closeOnOutsideClick);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("mousedown", closeOnOutsideClick);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  useEffect(() => {
    if (loading) close();
  }, [loading]);

  function openPicker() {
    setViewProvider(provider);
    setMobileStep("providers");
    setQuery("");
    setOpen(true);
  }

  function close() {
    setOpen(false);
    setQuery("");
  }

  function chooseProvider(nextProvider: ChatProvider) {
    setViewProvider(nextProvider);
    setMobileStep("models");
    setQuery("");
  }

  function chooseModel(nextModel: string) {
    if (!viewedProvider || viewedProvider.disabled) return;
    close();
    if (viewedProvider.value !== provider || nextModel !== model) {
      onChange(viewedProvider.value, nextModel);
    }
  }

  const triggerTitle = loading
    ? "Loading available providers and models"
    : streaming
      ? "Cannot change provider or model while streaming"
      : unavailableReason || "Choose provider and model";

  return (
    <div ref={rootRef} class="codex-agent-picker relative w-[220px] flex-none">
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
        <span class="sr-only">Provider and model</span>
        <span class="min-w-0 flex-1 truncate text-[11.5px] font-medium">
          {loading ? "Loading models…" : `${providerLabel} · ${modelLabel}`}
        </span>
        {!loading && <ChevronDown class="h-3 w-3 flex-none opacity-50" />}
      </button>

      {open && !loading && (
        <>
          <button
            type="button"
            class="fixed inset-0 z-40 cursor-default bg-black/55 md:hidden"
            aria-label="Close provider and model picker"
            onClick={close}
          />
          <div
            class="theme-menu-surface fixed inset-x-3 bottom-3 z-50 max-h-[calc(100dvh-1.5rem)] overflow-hidden rounded-panel border border-line bg-raised shadow-modal
                   md:absolute md:inset-x-auto md:bottom-full md:left-0 md:mb-2 md:w-[min(36rem,calc(100vw-1.5rem))] md:rounded-lg"
            role="dialog"
            aria-label="Choose provider and model"
          >
            <div class="flex h-11 items-center justify-between border-b border-line px-3">
              <div class="flex min-w-0 items-center gap-2">
                {mobileStep === "models" && (
                  <button
                    type="button"
                    onClick={() => setMobileStep("providers")}
                    class="-ml-1 rounded p-1 text-ink-300 hover:bg-tint-strong hover:text-ink-100 md:hidden"
                    aria-label="Back to providers"
                  >
                    <ChevronLeft class="h-4 w-4" />
                  </button>
                )}
                <span class="truncate text-[12px] font-semibold text-ink-100">
                  <span class="md:hidden">
                    {mobileStep === "providers" ? "Choose provider" : `${viewedProvider?.label || "Provider"} models`}
                  </span>
                  <span class="hidden md:inline">Provider and model</span>
                </span>
              </div>
              <button
                type="button"
                onClick={close}
                class="rounded p-1 text-ink-400 hover:bg-tint-strong hover:text-ink-100 md:hidden"
                aria-label="Close provider and model picker"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <div class="md:hidden">
              {mobileStep === "providers" ? (
                <ProviderList
                  options={providerOptions}
                  currentProvider={provider}
                  viewedProvider={viewProvider}
                  query={query}
                  onQueryChange={setQuery}
                  onChoose={chooseProvider}
                />
              ) : (
                <ModelList
                  provider={provider}
                  model={model}
                  viewedProvider={viewedProvider}
                  options={viewedModels}
                  onChoose={chooseModel}
                />
              )}
            </div>

            <div class="hidden min-h-0 grid-cols-[minmax(0,15rem)_minmax(0,1fr)] md:grid">
              <ProviderList
                options={providerOptions}
                currentProvider={provider}
                viewedProvider={viewProvider}
                query={query}
                onQueryChange={setQuery}
                onChoose={chooseProvider}
              />
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
      <div class="max-h-[min(22rem,calc(100dvh-8rem))] overflow-y-auto p-1.5 md:max-h-[20rem]" role="listbox" aria-label="Providers">
        {connected.length > 0 && (
          <ProviderSectionLabel>Connected</ProviderSectionLabel>
        )}
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

function ProviderSectionLabel({ children }: { children: string }) {
  return (
    <div class="px-2.5 pb-1 pt-0.5 text-[9.5px] font-semibold uppercase tracking-[0.12em] text-ink-500">
      {children}
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
      <div class="hidden border-b border-line px-3 py-2 md:block">
        <div class="truncate text-[11px] font-semibold text-ink-200">{viewedProvider.label} models</div>
        <div class="mt-0.5 text-[10px] text-ink-500">Choose one to apply this provider</div>
      </div>
      <div class="max-h-[min(22rem,calc(100dvh-8rem))] overflow-y-auto p-1.5 md:max-h-[16.5rem]" role="listbox" aria-label={`${viewedProvider.label} models`}>
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

function displayProvider(provider: string): string {
  return provider ? provider.charAt(0).toUpperCase() + provider.slice(1) : "Agent";
}
