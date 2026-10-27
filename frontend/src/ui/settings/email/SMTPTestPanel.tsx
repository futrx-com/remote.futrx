import { useState } from "preact/hooks";
import { Loader } from "../../primitives/icons";

export function SMTPTestPanel({
  defaultRecipient,
  testing,
  testMessage,
  onSendTest,
}: {
  defaultRecipient: string;
  testing: boolean;
  testMessage: string | null;
  onSendTest: (to: string) => void;
}) {
  const [testRecipient, setTestRecipient] = useState(defaultRecipient);

  return (
    <div class="px-3 pb-3 space-y-2 border-t border-line pt-3">
      <label class="block space-y-1.5">
        <span class="text-xs text-ink-300">Send a test email to</span>
        <input
          type="text"
          value={testRecipient}
          onInput={(e) => setTestRecipient((e.currentTarget as HTMLInputElement).value)}
          class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
        />
      </label>
      <button
        type="button"
        disabled={testing}
        onClick={() => onSendTest(testRecipient)}
        class="btn btn-secondary disabled:opacity-50 inline-flex items-center gap-2"
      >
        {testing && <Loader class="w-3.5 h-3.5 animate-spin" />}
        Send test email
      </button>
      {testMessage && <div class="text-xs text-ink-300">{testMessage}</div>}
    </div>
  );
}
