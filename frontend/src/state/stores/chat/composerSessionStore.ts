import { createStore } from "zustand/vanilla";
import type {
  ChatComposerSessionStoreActions,
  ChatComposerSessionStoreState,
  ComposerSessionStorage,
  PersistedComposerSession,
  QueuedPrompt,
} from "../../../models/chat";
import { SESSION_STORAGE_KEYS } from "../../../config/storageKeys.ts";

// Composer state is mirrored to sessionStorage so drafts and queued prompts
// survive a reload or navigation while an agent run (often a long stretch of
// tool calls) is still in flight. sessionStorage keeps it per-tab, matching
// the previous in-memory semantics; storage failures degrade to memory-only.
export function createChatComposerSessionStore(
  storage: ComposerSessionStorage | null = defaultStorage(),
) {
  const hydrated = hydrate(storage);
  return createStore<ChatComposerSessionStoreState & ChatComposerSessionStoreActions>()(
    (set) => ({
      ...hydrated,
      setDraft: (chatId, text) => set((state) => {
        const drafts = new Map(state.drafts);
        if (text) drafts.set(chatId, text);
        else drafts.delete(chatId);
        persist(storage, drafts, state.promptQueues);
        return { drafts };
      }),
      setQueuedPrompts: (chatId, prompts) => set((state) => {
        const promptQueues = new Map(state.promptQueues);
        if (prompts.length) promptQueues.set(chatId, prompts);
        else promptQueues.delete(chatId);
        persist(storage, state.drafts, promptQueues);
        return { promptQueues };
      }),
    }),
  );
}

function hydrate(storage: ComposerSessionStorage | null): Pick<
  ChatComposerSessionStoreState,
  "drafts" | "promptQueues"
> {
  const drafts = new Map<string, string>();
  const promptQueues = new Map<string, QueuedPrompt[]>();
  if (!storage) return { drafts, promptQueues };

  try {
    const raw = storage.getItem(SESSION_STORAGE_KEYS.composerSession);
    if (!raw) return { drafts, promptQueues };
    const parsed = JSON.parse(raw) as Partial<PersistedComposerSession> | null;
    for (const [chatId, text] of Object.entries(parsed?.drafts ?? {})) {
      if (typeof text === "string" && text) drafts.set(chatId, text);
    }
    for (const [chatId, prompts] of Object.entries(parsed?.queues ?? {})) {
      const valid = (Array.isArray(prompts) ? prompts : []).filter(
        (prompt): prompt is QueuedPrompt =>
          !!prompt && typeof prompt.id === "string" && typeof prompt.text === "string",
      );
      if (valid.length) promptQueues.set(chatId, valid);
    }
  } catch {
    // Corrupt or unreadable snapshot — start clean.
  }
  return { drafts, promptQueues };
}

function persist(
  storage: ComposerSessionStorage | null,
  drafts: ReadonlyMap<string, string>,
  promptQueues: ReadonlyMap<string, QueuedPrompt[]>,
): void {
  if (!storage) return;
  try {
    const snapshot: PersistedComposerSession = {
      drafts: Object.fromEntries(drafts),
      queues: Object.fromEntries(promptQueues),
    };
    storage.setItem(SESSION_STORAGE_KEYS.composerSession, JSON.stringify(snapshot));
  } catch {
    // Quota or privacy-mode failures fall back to in-memory behavior.
  }
}

function defaultStorage(): ComposerSessionStorage | null {
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
}

// ChatContainer remounts when the active chat changes, so composer state must
// outlive the component tree. Backed by per-tab sessionStorage so it also
// survives reloads within the browser session.
export const chatComposerSessionStore = createChatComposerSessionStore();
