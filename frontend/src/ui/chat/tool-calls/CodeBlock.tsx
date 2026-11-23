export function CodeBlock({ text, lang }: { text: string; lang?: string }) {
  return (
    <pre class={`overflow-x-auto touch-scroll-x p-3 text-[12.5px] leading-relaxed font-mono text-ink-100 ${lang === "diff" ? "" : ""}`}>
      {text}
    </pre>
  );
}
