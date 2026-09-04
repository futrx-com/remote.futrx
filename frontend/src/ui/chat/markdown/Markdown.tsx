import { useMemo } from "preact/hooks";
import { parseMarkdown } from "./blockParser";
import { highlightCode } from "./highlight";
import { renderInline } from "./inlineParser";
import type { MarkdownBlock } from "./types";
import { getTextAlignClass, isRtlText } from "./bidi";

export function Markdown({ children, chatId, cwd }: { children: string; chatId?: string; cwd?: string }) {
  const docIsRtl = isRtlText(children);
  const blocks = useMemo(() => parseMarkdown(children), [children]);
  const context: MarkdownRenderContext = { chatId, cwd, docIsRtl };
  return <>{blocks.map((block, index) => renderBlock(block, `md-${index}`, context))}</>;
}

interface MarkdownRenderContext {
  chatId?: string;
  cwd?: string;
  docIsRtl?: boolean;
  isRtl?: boolean;
}

function renderBlock(block: MarkdownBlock, key: string, context: MarkdownRenderContext) {
  switch (block.type) {
    case "paragraph":
      return renderParagraph(block.text, key, context);
    case "heading":
      return renderHeading(block.level, block.text, key, context);
    case "code":
      return renderCode(block, key);
    case "blockquote":
      return renderBlockquote(block, key, context);
    case "list":
      return renderList(block, key, context);
    case "table":
      return renderTable(block, key, context);
    case "hr":
      return <hr key={key} class="my-3 border-line" />;
  }
}

function renderParagraph(text: string, key: string, context: MarkdownRenderContext) {
  const isRtl = isRtlText(text);
  return (
    <p
      key={key}
      dir={isRtl ? "rtl" : "ltr"}
      class={`my-1.5 leading-relaxed break-words [overflow-wrap:anywhere] ${getTextAlignClass(text)}`}
    >
      {renderInline(text, key, { ...context, isRtl })}
    </p>
  );
}

function renderCode(block: Extract<MarkdownBlock, { type: "code" }>, key: string) {
  return (
    <div key={key} dir="ltr" class="relative my-3 rounded-lg border border-line bg-surface overflow-hidden text-left">
      {block.lang && (
        <div dir="ltr" class="px-3 py-1 text-[11px] text-ink-300 border-b border-line bg-tint text-left">
          {block.lang}
        </div>
      )}
      <pre dir="ltr" class="md-code overflow-x-auto touch-scroll p-3 text-[12.5px] leading-relaxed font-mono text-left">
        <code dir="ltr">{highlightCode(block.text, block.lang)}</code>
      </pre>
    </div>
  );
}

function renderBlockquote(
  block: Extract<MarkdownBlock, { type: "blockquote" }>,
  key: string,
  context: MarkdownRenderContext,
) {
  const hasRtl = block.children.some(
    (child) => "text" in child && typeof child.text === "string" && isRtlText(child.text),
  );
  const borderClass = hasRtl
    ? "border-r-2 border-accent-blue/45 pr-3 text-right"
    : "border-l-2 border-accent-blue/45 pl-3 text-left";
  const blockquoteContext = { ...context, isRtl: hasRtl };

  return (
    <blockquote
      key={key}
      dir={hasRtl ? "rtl" : "ltr"}
      class={`${borderClass} my-2 text-ink-200 italic min-w-0 break-words [overflow-wrap:anywhere]`}
    >
      {block.children.map((child, index) => renderBlock(child, `${key}-q-${index}`, blockquoteContext))}
    </blockquote>
  );
}

function renderTable(
  block: Extract<MarkdownBlock, { type: "table" }>,
  key: string,
  context: MarkdownRenderContext,
) {
  return (
    <div key={key} class="overflow-x-auto touch-scroll my-3 border border-line rounded-lg">
      <table class="w-full text-sm border-collapse">
        <thead class="bg-tint">
          <tr>
            {block.header.map((cell, index) => renderTableHeaderCell(cell, index, key, context))}
          </tr>
        </thead>
        <tbody>
          {block.rows.map((row, rowIndex) => (
            <tr key={rowIndex}>
              {row.map((cell, cellIndex) => renderTableDataCell(cell, cellIndex, rowIndex, key, context))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function renderTableHeaderCell(
  cell: string,
  index: number,
  blockKey: string,
  context: MarkdownRenderContext,
) {
  const isRtl = isRtlText(cell);
  return (
    <th
      key={index}
      dir={isRtl ? "rtl" : "ltr"}
      class={`${getTextAlignClass(cell)} px-3 py-1.5 font-semibold border-b border-line text-ink-100 [overflow-wrap:anywhere]`}
    >
      {renderInline(cell, `${blockKey}-h-${index}`, { ...context, isRtl })}
    </th>
  );
}

function renderTableDataCell(
  cell: string,
  cellIndex: number,
  rowIndex: number,
  blockKey: string,
  context: MarkdownRenderContext,
) {
  const isRtl = isRtlText(cell);
  return (
    <td
      key={cellIndex}
      dir={isRtl ? "rtl" : "ltr"}
      class={`${getTextAlignClass(cell)} px-3 py-1.5 border-b border-line [overflow-wrap:anywhere]`}
    >
      {renderInline(cell, `${blockKey}-r-${rowIndex}-${cellIndex}`, { ...context, isRtl })}
    </td>
  );
}

function renderHeading(level: 1 | 2 | 3 | 4 | 5 | 6, text: string, key: string, context: MarkdownRenderContext) {
  const isRtl = isRtlText(text);
  const dir = isRtl ? "rtl" : "ltr";
  const align = getTextAlignClass(text);
  const headingContext = { ...context, isRtl };
  if (level === 1) {
    return (
      <h1 key={key} dir={dir} class={`text-xl font-bold mt-3 mb-1.5 break-words [overflow-wrap:anywhere] ${align}`}>
        {renderInline(text, key, headingContext)}
      </h1>
    );
  }
  if (level === 2) {
    return (
      <h2 key={key} dir={dir} class={`text-lg font-bold mt-3 mb-1.5 break-words [overflow-wrap:anywhere] ${align}`}>
        {renderInline(text, key, headingContext)}
      </h2>
    );
  }
  return (
    <h3 key={key} dir={dir} class={`text-base font-bold mt-2 mb-1 break-words [overflow-wrap:anywhere] ${align}`}>
      {renderInline(text, key, headingContext)}
    </h3>
  );
}

function renderList(block: Extract<MarkdownBlock, { type: "list" }>, key: string, context: MarkdownRenderContext) {
  const isRtl = !!context.docIsRtl || block.items.some((item) => isRtlText(item.text));
  const dir = isRtl ? "rtl" : "ltr";
  const align = isRtl ? "text-right" : "text-left";
  const padding = isRtl ? "pr-5 pl-0 text-right" : "pl-5 pr-0 text-left";
  const className = `${block.ordered ? "list-decimal" : "list-disc"} list-outside ${padding} my-2 space-y-0.5`;
  const items = block.items.map((item, index) => {
    const content = renderInline(item.text, `${key}-li-${index}`, { ...context, isRtl });
    if (item.checked === undefined) {
      return <li key={index} dir={dir} class={align}>{content}</li>;
    }
    return (
      <li key={index} dir={dir} class={`list-none ${isRtl ? "-mr-5" : "-ml-5"} flex items-start gap-2 ${align}`}>
        <input type="checkbox" checked={item.checked} disabled class="mt-1 h-3.5 w-3.5 flex-none" />
        <span>{content}</span>
      </li>
    );
  });

  if (block.ordered) {
    return <ol key={key} dir={dir} class={className} start={block.start}>{items}</ol>;
  }
  return <ul key={key} dir={dir} class={className}>{items}</ul>;
}
