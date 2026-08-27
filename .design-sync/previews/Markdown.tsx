// Markdown — the agent-reply markdown renderer (headings, lists, code, tables).
import { Markdown } from "remote.futrx-web";

const richDoc = `# Deploy checklist

The staging build is green. Before promoting to production, run through the
steps below — the only manual one is rotating \`SESSION_SECRET\`.

## Steps

1. Merge \`ux-improvements\` into \`main\`
2. Run the migration with \`npm run db:migrate\`
3. Verify the health endpoint returns **200**

\`\`\`bash
curl -s https://staging.futrx.dev/healthz | jq .status
# => "ok"
\`\`\`

See the [runbook](https://example.com/runbook) for rollback instructions.`;

const tableAndQuote = `## Bundle size report

| Route | Before | After | Delta |
| --- | --- | --- | --- |
| \`/chat\` | 214 kB | 186 kB | **-28 kB** |
| \`/projects\` | 142 kB | 141 kB | -1 kB |
| \`/settings\` | 98 kB | 98 kB | ±0 |

> The \`/chat\` win comes from lazy-loading the terminal overlay — it only
> ships once the user actually opens a terminal.

---

Overall the gzipped total drops by *6.4%*.`;

const taskList = `### Migration progress

- [x] Export legacy chats to JSONL
- [x] Backfill \`project_id\` on orphaned threads
- [ ] Re-index message search
- [ ] Delete the compatibility shim

Unchecked items are scheduled for the next maintenance window.`;

const codeExplainer = `The race is in \`useChatReadMarker\` — the marker writes before the
scroll listener detaches:

\`\`\`tsx
useEffect(() => {
  const onScroll = () => markRead(chat.id);
  el.addEventListener("scroll", onScroll, { passive: true });
  return () => el.removeEventListener("scroll", onScroll);
}, [chat.id]);
\`\`\`

Swapping the cleanup order fixes it — no state change needed.`;

export const RichDocument = () => (
  <div className="w-full max-w-xl">
    <Markdown>{richDoc}</Markdown>
  </div>
);

export const TableAndQuote = () => (
  <div className="w-full max-w-xl">
    <Markdown>{tableAndQuote}</Markdown>
  </div>
);

export const TaskList = () => (
  <div className="w-full max-w-xl">
    <Markdown>{taskList}</Markdown>
  </div>
);

export const CodeExplainer = () => (
  <div className="w-full max-w-xl">
    <Markdown>{codeExplainer}</Markdown>
  </div>
);
