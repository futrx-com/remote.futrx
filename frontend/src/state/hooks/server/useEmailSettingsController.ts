import { useEffect, useState } from "preact/hooks";
import { emailApi } from "../../../api/emailApi";
import {
  smtpSettingsForm,
  type EmailProviderChoice,
  type SMTPFormInput,
} from "../../../model/email/application/smtpSettingsForm";
import type {
  AuthenticationMode,
  TLSMode,
} from "../../../config/constants/email-providers";
import type { PublicSMTPConfiguration } from "../../../model/email/api/smtpSettings";
import type { EmailSettingsGateway } from "../../../port/email/outbound/emailSettingsGateway";

export function useEmailSettingsController(gateway: EmailSettingsGateway = emailApi) {
  ////////////////////
  // Local State
  /////////////////////
  const [configuration, setConfiguration] = useState<PublicSMTPConfiguration | null>(null);
  const [form, setForm] = useState<SMTPFormInput>(smtpSettingsForm.blankGmailInput());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testing, setTesting] = useState(false);
  const [testMessage, setTestMessage] = useState<string | null>(null);

  /////////////////////
  // Field setters
  ////////////////////
  function update(patch: Partial<SMTPFormInput>) {
    setForm((current) => ({ ...current, ...patch }));
  }

  function selectProvider(providerId: EmailProviderChoice) {
    if (providerId === "gmail") {
      update({
        ...smtpSettingsForm.blankGmailInput(),
        username: form.fromAddress,
        fromAddress: form.fromAddress,
      });
    } else {
      update({ ...smtpSettingsForm.blankCustomInput(), fromAddress: form.fromAddress });
    }
  }

  function setFromAddress(value: string) {
    // Gmail links the sender address and username until the admin edits the
    // username explicitly.
    if (form.providerId === "gmail" && form.username === form.fromAddress) {
      update({ fromAddress: value, username: value });
    } else {
      update({ fromAddress: value });
    }
  }

  function setHost(value: string) {
    update({ host: value });
  }
  function setPort(value: string) {
    update({ port: value });
  }
  function setTlsMode(value: TLSMode) {
    update(
      value === "none" ? { tlsMode: value, authentication: "none" } : { tlsMode: value }
    );
  }
  function setAuthentication(value: AuthenticationMode) {
    update({ authentication: value });
  }
  function setUsername(value: string) {
    update({ username: value });
  }
  function setPassword(value: string) {
    update({ password: value });
  }

  /////////////////////
  // Handlers
  ////////////////////
  async function save(event: Event) {
    event.preventDefault();
    const prepared = smtpSettingsForm.prepareSubmission(form);
    if (!prepared.valid) {
      setError(prepared.error);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const settings = await gateway.save(prepared.request);
      setConfiguration(settings.configuration ?? null);
      if (settings.configuration) {
        update({ ...smtpSettingsForm.toInput(settings.configuration), password: "" });
      }
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function sendTest(to: string) {
    setTesting(true);
    setTestMessage(null);
    try {
      await gateway.sendTest(to);
      setTestMessage(`Test email sent to ${to}.`);
    } catch (cause) {
      setTestMessage((cause as Error).message);
    } finally {
      setTesting(false);
    }
  }

  async function remove() {
    setError(null);
    try {
      await gateway.remove();
      setConfiguration(null);
      setForm(smtpSettingsForm.blankGmailInput());
    } catch (cause) {
      setError((cause as Error).message);
    }
  }

  ///////////////////
  // Effects
  //////////////////
  useEffect(() => {
    let cancelled = false;
    gateway
      .get()
      .then((settings) => {
        if (cancelled) return;
        setConfiguration(settings.configuration ?? null);
        if (settings.configuration) {
          setForm(smtpSettingsForm.toInput(settings.configuration));
        }
      })
      .catch((cause) => !cancelled && setError((cause as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    configuration,
    configured: configuration !== null,
    error,
    form,
    loading,
    remove,
    save,
    saving,
    selectProvider,
    sendTest,
    setAuthentication,
    setFromAddress,
    setHost,
    setPassword,
    setPort,
    setTlsMode,
    setUsername,
    testMessage,
    testing,
  };
}
