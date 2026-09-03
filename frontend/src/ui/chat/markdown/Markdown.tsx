import { useMemo } from "preact/hooks";
import { parseMarkdown } from "./blockParser";
import { highlightCode } from "./highlight";
import { renderInline } from "./inlineParser";
import type { MarkdownBlock } from "./types";

export function Markdown({ children, chatId, cwd }: { children: string; chatId?: string; cwd?: string }) {
  const blocks = useMemo(() => parseMarkdown(children), [children]);
  const context = { chatId, cwd };
  return <>{blocks.map((block, index) => renderBlock(block, `md-${index}`, context))}</>;
}

interface MarkdownRenderContext {
  chatId?: string;
  cwd?: string;
}

function renderBlock(block: MarkdownBlock, key: string, context: MarkdownRenderContext) {
  switch (block.type) {
    case "paragraph":
      return <p key={key} class="my-1.5 leading-relaxed">{renderInline(block.text, key, context)}</p>;
    case "heading":
      return renderHeading(block.level, block.text, key, context);
    case "code":
      return (
        <div key={key} class="relative my-3 rounded-lg border border-line bg-surface overflow-hidden">
          {block.lang && (
            <div class="px-3 py-1 text-[11px] text-ink-300 border-b border-line bg-tint">
              {block.lang}
            </div>
          )}
          <pre class="md-code overflow-x-auto touch-scroll p-3 text-[12.5px] leading-relaxed font-mono">
            <code>{highlightCode(block.text, block.lang)}</code>
          </pre>
        </div>
      );
    case "blockquote":
      return (
        <blockquote key={key} class="border-l-2 border-accent-blue/45 pl-3 my-2 text-ink-200 italic">
          {block.children.map((child, index) => renderBlock(child, `${key}-q-${index}`, context))}
        </blockquote>
      );
    case "list":
      return renderList(block, key, context);
    case "table":
      return (
        <div key={key} class="overflow-x-auto touch-scroll my-3 border border-line rounded-lg">
          <table class="w-full text-sm border-collapse">
            <thead class="bg-tint">
              <tr>
                {block.header.map((cell, index) => (
                  <th key={index} class="text-left px-3 py-1.5 font-semibold border-b border-line text-ink-100">
                    {renderInline(cell, `${key}-h-${index}`, context)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {block.rows.map((row, rowIndex) => (
                <tr key={rowIndex}>
                  {row.map((cell, cellIndex) => (
                    <td key={cellIndex} class="px-3 py-1.5 border-b border-line">
                      {renderInline(cell, `${key}-r-${rowIndex}-${cellIndex}`, context)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "hr":
      return <hr key={key} class="my-3 border-line" />;
  }
}

function renderHeading(level: 1 | 2 | 3 | 4 | 5 | 6, text: string, key: string, context: MarkdownRenderContext) {
  if (level === 1) return <h1 key={key} class="text-xl font-bold mt-3 mb-1.5">{renderInline(text, key, context)}</h1>;
  if (level === 2) return <h2 key={key} class="text-lg font-bold mt-3 mb-1.5">{renderInline(text, key, context)}</h2>;
  return <h3 key={key} class="text-base font-bold mt-2 mb-1">{renderInline(text, key, context)}</h3>;
}

function renderList(block: Extract<MarkdownBlock, { type: "list" }>, key: string, context: MarkdownRenderContext) {
  const className = `${block.ordered ? "list-decimal" : "list-disc"} list-outside pl-5 my-2 space-y-0.5`;
  const items = block.items.map((item, index) => {
    const content = renderInline(item.text, `${key}-li-${index}`, context);
    if (item.checked === undefined) return <li key={index}>{content}</li>;
    return (
      <li key={index} class="list-none -ml-5 flex items-start gap-2">
        <input
          type="checkbox"
          checked={item.checked}
          disabled
          class="mt-1 h-3.5 w-3.5 flex-none"
        />
        <span>{content}</span>
      </li>
    );
  });

  if (block.ordered) {
    return <ol key={key} class={className} start={block.start}>{items}</ol>;
  }
  return <ul key={key} class={className}>{items}</ul>;
}
