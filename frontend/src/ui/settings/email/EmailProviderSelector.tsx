import type { EmailProviderChoice } from "../../../model/email/application/smtpSettingsForm";
import { Check } from "../../primitives/icons";

export function EmailProviderSelector({
  providerId,
  onSelect,
}: {
  providerId: EmailProviderChoice;
  onSelect: (id: EmailProviderChoice) => void;
}) {
  return (
    <div class="flex gap-2">
      <button
        type="button"
        onClick={() => onSelect("gmail")}
        class={`flex-1 rounded-md border px-3 py-2 text-left text-xs ${
          providerId === "gmail"
            ? "border-accent-blue bg-tint text-ink-100"
            : "border-line text-ink-300"
        }`}
      >
        <div class="flex items-center gap-1.5 font-medium">
          Gmail
          <span class="inline-flex items-center gap-0.5 text-[10px] text-accent-green">
            <Check class="w-3 h-3" /> Recommended
          </span>
        </div>
        <div class="text-[11px] text-ink-300 mt-0.5">Send through your Google account.</div>
      </button>
      <button
        type="button"
        onClick={() => onSelect("custom")}
        class={`flex-1 rounded-md border px-3 py-2 text-left text-xs ${
          providerId === "custom"
            ? "border-accent-blue bg-tint text-ink-100"
            : "border-line text-ink-300"
        }`}
      >
        <div class="font-medium">Custom SMTP</div>
        <div class="text-[11px] text-ink-300 mt-0.5">Any STARTTLS, implicit TLS, or relay server.</div>
      </button>
    </div>
  );
}
