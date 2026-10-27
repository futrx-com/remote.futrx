import type { ReleaseNotesController } from "../../../state/hooks/server/useReleaseNotes";
import { Markdown } from "../../chat/markdown/Markdown";
import { Loader } from "../../primitives/icons";

export function ReleaseNotes({ tag, controller }: {
  tag: string;
  controller: ReleaseNotesController;
}) {
  const { notes, loading, error, retry } = controller;
  return (
    <section class="rounded-card border border-line bg-surface p-4" aria-label={`Release notes for ${tag}`}>
      <h3 class="text-[13.5px] font-semibold text-ink-50">Release notes · {tag}</h3>
      {loading ? (
        <p role="status" class="mt-3 flex items-center gap-2 text-[12.5px] text-ink-300">
          <Loader class="h-3.5 w-3.5 animate-spin" /> Loading release notes…
        </p>
      ) : error ? (
        <div class="mt-3 flex flex-wrap items-center gap-2 text-[12.5px] text-ink-300">
          <span role="status">{error}</span>
          <button type="button" onClick={retry} class="text-accent-blue hover:underline">Retry release notes</button>
        </div>
      ) : notes?.body ? (
        <div tabIndex={0} aria-label="Release notes content" class="mt-3 max-h-80 overflow-y-auto touch-scroll text-[13px] leading-relaxed text-ink-200 break-words">
          <Markdown>{notes.body}</Markdown>
        </div>
      ) : (
        <p class="mt-3 text-[12.5px] text-ink-300">No release notes were included with this version.</p>
      )}
    </section>
  );
}
