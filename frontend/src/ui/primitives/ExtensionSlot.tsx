// Renders the contributions an image's `ui/` extension registered for one
// slot. Each contribution owns a plain <div> and draws into it with DOM APIs,
// so extension code never touches the SPA's component tree.

import { useEffect, useMemo, useRef } from "preact/hooks";
import type {
  ExtensionRender,
  ExtensionSlotContext,
  ExtensionSlotName,
} from "../../models/extension";
import { useExtensionContributions } from "../../state/hooks/extensions/useExtensionContributions";

export type ExtensionSlotProps = Omit<ExtensionSlotContext, "slot"> & {
  name: ExtensionSlotName;
  class?: string;
};

export function ExtensionSlot({
  name,
  class: className,
  scope,
  instance,
  projectId,
  projectName,
  chatId,
  cwd,
}: ExtensionSlotProps) {
  const contextKey = JSON.stringify({
    scope,
    instance,
    projectId,
    projectName,
    chatId,
    cwd,
  });
  const context = useMemo<ExtensionSlotContext>(
    () => ({ slot: name, scope, instance, projectId, projectName, chatId, cwd }),
    [name, contextKey],
  );
  const contributions = useExtensionContributions(name, context);

  if (!contributions.length) return null;

  return (
    <>
      {contributions.map((contribution) => (
        <ContributionHost
          key={contribution.id}
          class={className}
          renderContribution={contribution.render}
          context={context}
        />
      ))}
    </>
  );
}

function ContributionHost({
  renderContribution,
  context,
  class: className,
}: {
  renderContribution: ExtensionRender;
  context: ExtensionSlotContext;
  class?: string;
}) {
  const hostRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    let cleanup: void | (() => void);
    // An extension that throws while rendering loses its own node and nothing
    // else: the surrounding surface must still paint.
    try {
      cleanup = renderContribution(host, context);
    } catch (error) {
      console.error("[extensions] slot render failed", context.slot, error);
    }
    return () => {
      try {
        if (typeof cleanup === "function") cleanup();
      } catch (error) {
        console.error("[extensions] slot cleanup failed", context.slot, error);
      }
      host.replaceChildren();
    };
  }, [renderContribution, context]);

  return <div ref={hostRef} class={className ?? "contents"} />;
}
