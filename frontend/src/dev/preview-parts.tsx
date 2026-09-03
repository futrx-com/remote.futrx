// Scratch harness for visual review only. Not referenced by the app entry.
import { render } from "preact";
import { useRef } from "preact/hooks";
import { NoChatSelected } from "../ui/layout/NoChatSelected";
import { AppearanceSettings } from "../ui/settings/AppearanceSettings";
import { Empty, Field, Grid, Panel } from "../ui/projects/project-containers/ProjectContainerPrimitives";
import { CreateProjectModal } from "../ui/projects/CreateProjectModal";
import { ToolShell } from "../ui/chat/tool-calls/ToolShell";
import { CodeBlock } from "../ui/chat/tool-calls/CodeBlock";
import { ModelPicker } from "../ui/chat/header/ModelPicker";
import { ErrorMessage } from "../ui/chat/messages/ErrorMessage";
import { ThinkingBlock } from "../ui/chat/messages/ThinkingBlock";
import { UsagePill } from "../ui/chat/header/UsagePill";
import { TerminalIcon } from "../ui/primitives/icons";
import "../index.css";

const noop = () => {};

function Section({ title, children }: { title: string; children: unknown }) {
  return (
    <section class="space-y-3">
      <h2 class="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-400">{title}</h2>
      {children as never}
    </section>
  );
}

function Parts() {
  const modelRef = useRef<HTMLDivElement>(null);
  return (
    <div class="h-full overflow-y-auto bg-app p-6">
      <div class="mx-auto grid max-w-[1200px] gap-8 md:grid-cols-2">
        <Section title="Empty workspace">
          <div class="h-[420px] overflow-hidden rounded-panel border border-line bg-canvas">
            <NoChatSelected hasProjects onNewProject={noop} onNewChat={noop} onHamburger={noop} />
          </div>
        </Section>

        <Section title="Settings section">
          <AppearanceSettings theme="dark" loading={false} saving={false} error={null} onThemeChange={noop} />
          <div class="flex items-start gap-2">
            <UsagePill
              totals={{ inputTokens: 18422, outputTokens: 3120, cacheReadTokens: 90210, cacheWriteTokens: 1200 }}
              tokenLabel="112.9k"
              costUsd={0.4213}
            />
            <ModelPicker
              modelRef={modelRef}
              open
              model="claude-opus-5"
              streaming={false}
              displayLabel={() => "Claude Opus 5"}
              options={[
                { value: "claude-opus-5", label: "Claude Opus 5", sub: "Deep reasoning" },
                { value: "claude-sonnet-5", label: "Claude Sonnet 5", sub: "Balanced" },
                { value: "claude-fable-5", label: "Claude Fable 5", sub: "Fast" },
              ]}
              onToggle={noop}
              onPick={noop}
            />
          </div>
        </Section>

        <Section title="Container panels">
          <Panel title="Resources">
            <Grid>
              <Field label="Memory" value="812 MB / 4 GB" />
              <Field label="Disk" value="6.4 GB" />
              <Field label="Container" value="rf-gamerhead" mono />
              <Field label="State" value="Missing — needs reprovision" tone="warn" />
            </Grid>
          </Panel>
          <Empty text="No secrets configured for this project yet." />
          <ErrorMessage message="Container rf-gamerhead is unreachable over LXD." />
        </Section>

        <Section title="Tool calls">
          <ToolShell
            icon={<TerminalIcon class="h-4 w-4" />}
            label={<span class="font-medium">journalctl -u gamerhead-queue</span>}
            badge="exit 0"
            status="done"
            defaultOpen
          >
            <CodeBlock text={"Aug 28 04:11:02 systemd[1]: Started gamerhead queue worker.\nAug 28 04:11:09 php[24812]: PHP Fatal error:  Uncaught RedisException"} />
          </ToolShell>
          <ToolShell
            icon={<TerminalIcon class="h-4 w-4" />}
            label={<span class="font-medium">2 tools used</span>}
            status="running"
          />
          <ToolShell
            icon={<TerminalIcon class="h-4 w-4" />}
            label={<span class="font-medium">npm run build</span>}
            badge="exit 1"
            status="done"
            isError
          />
        </Section>

        <Section title="Reasoning">
          <ThinkingBlock
            text="I should inspect the existing flow before choosing the smallest safe change."
            active={false}
          />
          <ThinkingBlock
            text="The provider is still streaming this reasoning segment into the same disclosure."
            active
          />
        </Section>
      </div>

      {new URLSearchParams(location.search).get("modal") === "1" && (
        <CreateProjectModal open projects={[]} onClose={noop} onCreate={async () => {}} />
      )}
    </div>
  );
}

document.documentElement.dataset.theme =
  new URLSearchParams(location.search).get("theme") === "light" ? "light" : "dark";
render(<Parts />, document.getElementById("root")!);
