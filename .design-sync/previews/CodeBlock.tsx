// CodeBlock — bare mono <pre> used inside tool-call bodies; frame it like the app does.
import { CodeBlock } from "remote.futrx-web";

const tsSource = `export function parseUnifiedDiff(diff: string): DiffFile[] {
  const files: DiffFile[] = [];
  let file: DiffFile | null = null;

  for (const line of diff.split("\\n")) {
    if (line.startsWith("diff --git ")) {
      file = makeFile(parseGitHeaderPaths(line));
      files.push(file);
    }
  }
  return files;
}`;

const shellOutput = `$ git log --oneline -4
9172232 feat(pwa): install the web app and receive notifications
9bb4ec2 feat(push): notify when an agent needs you
ff95e7a feat(webpush): add a standard-library Web Push client
de50101 feat(chat): tag events emitted by scheduled runs`;

const longLine = `const sessionKey = await crypto.subtle.importKey("raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign", "verify"]);
const signature = await crypto.subtle.sign("HMAC", sessionKey, payload);`;

const Frame = ({ children }: { children?: any }) => (
  <div className="w-full max-w-xl">
    <div className="rounded-lg border border-white/10 bg-white/[0.04] overflow-hidden">
      {children}
    </div>
  </div>
);

export const TypeScript = () => (
  <Frame>
    <CodeBlock text={tsSource} lang="ts" />
  </Frame>
);

export const ShellOutput = () => (
  <Frame>
    <CodeBlock text={shellOutput} />
  </Frame>
);

export const LongLinesScroll = () => (
  <Frame>
    <CodeBlock text={longLine} lang="ts" />
  </Frame>
);
