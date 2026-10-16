import { useEffect, useRef, useState } from "preact/hooks";
import type { PasswordInviteResult } from "../../models/user";
import {
  invitationDeliveryCopy,
  type InvitationDeliveryTone,
} from "../../state/hooks/users/invitationDeliveryCopy";
import { useConfirm } from "../../state/context/ConfirmContext";
import {
  invitationCredentialFileService,
  UnsupportedCredentialTextError,
} from "../../services/users/invitationCredentialFileService";
import { fileDownloadService } from "../../services/platform/fileDownloadService";
import { Download } from "../primitives/icons";

const TONE_CLASS: Record<InvitationDeliveryTone, string> = {
  success: "border-accent-green/30 bg-accent-green/[0.08] text-accent-green",
  neutral: "border-line bg-tint text-ink-100",
  warning: "border-accent-yellow/30 bg-accent-yellow/[0.08] text-accent-yellow",
  error: "border-accent-red/30 bg-accent-red/[0.08] text-accent-red",
};

const BUTTON_CLASS =
  "h-8 px-2.5 rounded bg-tint-strong hover:bg-tint-active text-ink-100 text-[12px] font-medium inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed";

// The result of a password invite or reset, shown inline like the recovery
// codes. The temporary password is shown only here, from the props the parent
// holds in component state; nothing in this card writes it anywhere else.
// Closing the card (Done) or unmounting it drops the parent's copy.
export function IssuedCredentialsCard({
  result,
  onDone,
}: {
  result: PasswordInviteResult;
  onDone: () => void;
}) {
  const confirm = useConfirm();
  const copy = invitationDeliveryCopy.describe(result);
  const [revealed, setRevealed] = useState(copy.credentialsVisible);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const mounted = useRef(true);

  const { email } = result.user;
  const pdfUnsupported = invitationCredentialFileService.unsupportedReason({ email });

  // An emailed invitation hides the credentials behind a question: the user
  // already has them, so seeing them again is the admin's choice.
  useEffect(() => {
    mounted.current = true;
    if (copy.revealQuestion) {
      void confirm({
        title: "Show the credentials?",
        description: `The invitation was emailed to ${email}.`,
        message: copy.revealQuestion,
        confirmLabel: "View credentials",
        cancelLabel: "Not now",
        tone: "neutral",
      }).then((yes) => {
        if (yes && mounted.current) setRevealed(true);
      });
    }
    return () => {
      mounted.current = false;
    };
    // The card is keyed by the issue, so this runs once per result.
  }, []);

  function downloadPdf() {
    setDownloadError(null);
    try {
      const file = invitationCredentialFileService.build(
        { email, temporaryPassword: result.temporaryPassword, signInUrl: location.origin },
        new Date()
      );
      fileDownloadService.save(file.content, file.filename, file.mimeType);
    } catch (cause) {
      setDownloadError(
        cause instanceof UnsupportedCredentialTextError
          ? cause.message
          : "Could not create the PDF."
      );
    }
  }

  return (
    <div role="status" class={`rounded-md border p-3 space-y-2.5 ${TONE_CLASS[copy.tone]}`}>
      <div class="text-[13px] font-medium break-words">{copy.message}</div>

      {revealed && (
        <>
          <div class="rounded border border-line bg-inset px-2.5 py-2 space-y-1.5 text-[12.5px] text-ink-100">
            <CredentialLine label="Email" value={email} />
            <CredentialLine label="Temporary password" value={result.temporaryPassword} mono />
          </div>
          <div class="text-[11.5px] text-ink-300 leading-snug">
            This password is shown only now. They will choose a new one the first time they sign in.
            To get a new temporary password later, use Reset on their row.
          </div>
        </>
      )}

      <div class="flex flex-wrap items-center gap-2">
        {revealed ? (
          <button
            type="button"
            onClick={downloadPdf}
            disabled={pdfUnsupported !== null}
            title={pdfUnsupported ?? undefined}
            class={BUTTON_CLASS}
          >
            <Download class="w-3.5 h-3.5" /> Download PDF
          </button>
        ) : (
          <button type="button" onClick={() => setRevealed(true)} class={BUTTON_CLASS}>
            View credentials
          </button>
        )}
        <button type="button" onClick={onDone} class={BUTTON_CLASS}>
          Done
        </button>
      </div>
      {revealed && pdfUnsupported && (
        <div class="text-[11.5px] text-ink-300 leading-snug">{pdfUnsupported}</div>
      )}
      {downloadError && <div class="text-[11.5px] text-accent-red">{downloadError}</div>}
    </div>
  );
}

function CredentialLine({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div class="flex items-center gap-2">
      <span class="text-ink-400 w-36 flex-none">{label}</span>
      <code class={`flex-1 min-w-0 break-all select-all ${mono ? "font-mono text-[13px]" : ""}`}>
        {value}
      </code>
      <CopyButton value={value} label={label} />
    </div>
  );
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      return;
    }
    setCopied(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1500);
  }

  return (
    <button
      type="button"
      onClick={() => void copy()}
      aria-label={`Copy ${label.toLowerCase()}`}
      class="text-[11px] text-ink-300 hover:text-ink-100 flex-none"
    >
      {copied ? "copied" : "copy"}
    </button>
  );
}
