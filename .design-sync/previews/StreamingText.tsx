// StreamingText — typewriter renderer for assistant prose; renders Markdown once settled.
import { StreamingText } from "remote.futrx-web";

export const RenderedMarkdown = () => (
  <div className="w-full max-w-xl text-[15px] leading-7 text-ink-100">
    <StreamingText
      streaming={false}
      chatId="chat_9f2c"
      cwd="~/dev/remote.futrx"
      text={`Push notifications are wired up. The flow:

1. The service worker registers on first load.
2. \`subscribeToPush()\` posts the subscription to \`/api/push/subscribe\`.
3. The server signs payloads with the VAPID key pair.

To test locally, run:

\`\`\`bash
node scripts/send-test-push.mjs --topic agent-needs-you
\`\`\`

Safari requires the app to be installed to the Home Screen before \`Notification.requestPermission()\` resolves.`}
    />
  </div>
);

export const ShortAnswer = () => (
  <div className="w-full max-w-xl text-[15px] leading-7 text-ink-100">
    <StreamingText
      streaming={false}
      text="Done — the composer now keeps focus after sending, and Escape clears the draft."
    />
  </div>
);

export const Streaming = () => (
  <div className="w-full max-w-xl text-[15px] leading-7 text-ink-100">
    <StreamingText
      streaming={true}
      text="Looking at the scheduler next — the cron parser accepts the new syntax, so"
    />
  </div>
);
