// ToolShell — collapsible frame every tool call renders inside.
import { ToolShell, CodeBlock, Terminal } from "remote.futrx-web";

const output = `$ npm run build
> remote.futrx-web@0.3.0 build
> tsc -b && vite build

vite v8.0.5 building for production...
✓ 214 modules transformed.
dist/index.html   0.46 kB
✓ built in 3.42s`;

export const Done = () => (
  <div className="w-full max-w-xl">
    <ToolShell
      icon={<Terminal class="w-4 h-4" />}
      label="npm run build"
      badge="exit 0"
      status="done"
      defaultOpen
    >
      <CodeBlock text={output} />
    </ToolShell>
  </div>
);

export const Running = () => (
  <div className="w-full max-w-xl">
    <ToolShell
      icon={<Terminal class="w-4 h-4" />}
      label="npm test — 42 suites"
      status="running"
    />
  </div>
);

export const Failed = () => (
  <div className="w-full max-w-xl">
    <ToolShell
      icon={<Terminal class="w-4 h-4" />}
      label="npm run lint"
      badge="exit 1"
      status="done"
      isError
      defaultOpen
    >
      <CodeBlock text={`src/ui/chat/ChatThread.tsx
  12:7  error  'threadId' is assigned a value but never used`} />
    </ToolShell>
  </div>
);

export const Collapsed = () => (
  <div className="w-full max-w-xl">
    <ToolShell
      icon={<Terminal class="w-4 h-4" />}
      label="git status"
      badge="1.2s"
      status="done"
    >
      <CodeBlock text="On branch main — nothing to commit, working tree clean" />
    </ToolShell>
  </div>
);
