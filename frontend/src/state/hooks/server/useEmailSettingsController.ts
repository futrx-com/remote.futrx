import { useEffect, useState } from "preact/hooks";
import { emailApi } from "../../../api/emailApi";
import type { EmailSettings } from "../../../models/email";
import { emailSettingsForm } from "./emailSettingsForm";

export function useEmailSettingsController() {
  ////////////////////
  // Local State
  /////////////////////
  const [settings, setSettings] = useState<EmailSettings | null>(null);
  const [address, setAddress] = useState("");
  const [appPassword, setAppPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testing, setTesting] = useState(false);
  const [testMessage, setTestMessage] = useState<string | null>(null);

  /////////////////////
  // Handlers
  ////////////////////
  async function save(event: Event) {
    event.preventDefault();
    const prepared = emailSettingsForm.prepareSubmission({ address, appPassword });
    if (!prepared.valid) {
      setError(prepared.error);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const value = await emailApi.save(prepared.address, prepared.appPassword);
      setSettings(value);
      setAddress(value.address);
      setAppPassword("");
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
      await emailApi.sendTest(to);
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
      await emailApi.remove();
      setSettings({ configured: false, address: "" });
      setAddress("");
      setAppPassword("");
    } catch (cause) {
      setError((cause as Error).message);
    }
  }

  ///////////////////
  // Effects
  //////////////////
  useEffect(() => {
    let cancelled = false;
    emailApi
      .get()
      .then((value) => {
        if (cancelled) return;
        setSettings(value);
        setAddress(value.address);
      })
      .catch((cause) => !cancelled && setError((cause as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    address,
    appPassword,
    error,
    loading,
    remove,
    save,
    saving,
    sendTest,
    setAddress,
    setAppPassword,
    settings,
    testMessage,
    testing,
  };
}
