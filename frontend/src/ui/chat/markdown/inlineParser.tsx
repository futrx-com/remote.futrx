import type { ComponentChildren } from "preact";
import { mediaViewerStore } from "../../../state/stores/media/mediaViewerStore";
import { fileService } from "../../../services/files/fileService.ts";
import { internalPathOpenUrl } from "../ideLinks";

const urlPattern = /^https?:\/\/[^\s<]+/;

export function renderInline(text: string, keyPrefix: string, context: InlineRenderContext = {}): ComponentChildren[] {
  const nodes: ComponentChildren[] = [];
  let plain = "";
  let index = 0;

  const flush = () => {
    if (plain) {
      nodes.push(plain);
      plain = "";
    }
  };

  const addWrapped = (tag: "strong" | "em" | "del", content: string, markerLength: number, end: number) => {
    flush();
    const key = `${keyPrefix}-${nodes.length}`;
    const children = renderInline(content, key, context);
    if (tag === "strong") nodes.push(<strong key={key}>{children}</strong>);
    if (tag === "em") nodes.push(<em key={key}>{children}</em>);
    if (tag === "del") nodes.push(<del key={key}>{children}</del>);
    index = end + markerLength;
  };

  while (index < text.length) {
    if (text[index] === "`") {
      const end = text.indexOf("`", index + 1);
      if (end > index + 1) {
        flush();
        nodes.push(
          <code key={`${keyPrefix}-${nodes.length}`} class="bg-tint-strong text-ink-100 px-1 py-0.5 rounded text-[12.5px] font-mono">
            {text.slice(index + 1, end)}
          </code>
        );
        index = end + 1;
        continue;
      }
    }

    if (text.startsWith("**", index)) {
      const end = text.indexOf("**", index + 2);
      if (end > index + 2) {
        addWrapped("strong", text.slice(index + 2, end), 2, end);
        continue;
      }
    }

    if (text.startsWith("~~", index)) {
      const end = text.indexOf("~~", index + 2);
      if (end > index + 2) {
        addWrapped("del", text.slice(index + 2, end), 2, end);
        continue;
      }
    }

    if (text[index] === "*" && text[index + 1] !== "*") {
      const end = text.indexOf("*", index + 1);
      if (end > index + 1) {
        addWrapped("em", text.slice(index + 1, end), 1, end);
        continue;
      }
    }

    if (text[index] === "[") {
      const labelEnd = text.indexOf("]", index + 1);
      const hrefStart = labelEnd >= 0 ? labelEnd + 1 : -1;
      if (hrefStart >= 0 && text[hrefStart] === "(") {
        const hrefEnd = text.indexOf(")", hrefStart + 1);
        if (hrefEnd > hrefStart + 1) {
          const href = safeHref(text.slice(hrefStart + 1, hrefEnd), context);
          if (href) {
            flush();
            const key = `${keyPrefix}-${nodes.length}`;
            nodes.push(
              <a
                key={key}
                href={href}
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent-blue hover:underline"
                onClick={(event) => maybeOpenMediaViewer(event, href)}
              >
                {renderInline(text.slice(index + 1, labelEnd), key, context)}
              </a>
            );
            index = hrefEnd + 1;
            continue;
          }
        }
      }
    }

    const url = text.slice(index).match(urlPattern)?.[0];
    if (url) {
      const href = trimTrailingUrlPunctuation(url);
      flush();
      nodes.push(
        <a key={`${keyPrefix}-${nodes.length}`} href={href} target="_blank" rel="noopener noreferrer" class="text-accent-blue hover:underline">
          {href}
        </a>
      );
      index += href.length;
      continue;
    }

    plain += text[index];
    index++;
  }

  flush();
  return nodes;
}

interface InlineRenderContext {
  chatId?: string;
  cwd?: string;
}

function safeHref(raw: string, context: InlineRenderContext): string | null {
  const href = raw.trim();
  const internalHref = internalPathOpenUrl(href, context);
  if (internalHref) return internalHref;
  if (
    href.startsWith("https://") ||
    href.startsWith("http://") ||
    href.startsWith("mailto:") ||
    href.startsWith("/") ||
    href.startsWith("#")
  ) {
    return href;
  }
  return null;
}

function trimTrailingUrlPunctuation(url: string): string {
  return url.replace(/[),.;:!?]+$/, "");
}

// Media links produced by internalPathOpenUrl point at the inline media-open
// endpoint; render those in the in-app viewer instead of a new tab. Modified
// clicks (cmd/ctrl/shift/middle) keep the browser's default behavior.
function maybeOpenMediaViewer(event: MouseEvent, href: string): void {
  if (event.defaultPrevented) return;
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  if (!href.includes("/media-open?")) return;
  const name = mediaOpenFileName(href);
  const kind = name ? fileService.viewableMediaKind(name) : null;
  if (!name || !kind) return;
  event.preventDefault();
  mediaViewerStore.getState().open({ url: href, name, kind });
}

function mediaOpenFileName(href: string): string {
  try {
    const url = new URL(href, window.location.origin);
    const path = url.searchParams.get("path") || "";
    const base = path.split("/").pop() || "";
    // Paths may carry :line or :line:column suffixes — strip them for display.
    return base.replace(/(:\d+){1,2}$/, "");
  } catch {
    return "";
  }
}
