import { useEffect, useState } from "preact/hooks";
import { googleOAuthApi } from "../../../api/authApi";
import type { GoogleOAuthSettings } from "../../../models/auth";
import { useAuthContext } from "../../context/AuthContext";

export function useGoogleOAuthSettingsController() {
  ////////////////////
  // Local State
  /////////////////////
  const [settings, setSettings] = useState<GoogleOAuthSettings | null>(null);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  ////////////////////
  // Global State
  /////////////////////
  const { auth } = useAuthContext();


  /////////////////////
  // Handlers
  ////////////////////
  async function save(event: Event) {
    event.preventDefault();
    if (!clientId.trim() || !clientSecret.trim()) {
      setError("Both the Google client ID and client secret are required.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const value = await googleOAuthApi.save(clientId.trim(), clientSecret.trim());
      setSettings(value);
      setClientId(value.clientId);
      setClientSecret("");
      await auth.refresh();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }


  ///////////////////
  // Effects
  //////////////////
  useEffect(() => {
    let cancelled = false;
    googleOAuthApi.get()
      .then((value) => {
        if (cancelled) return;
        setSettings(value);
        setClientId(value.clientId);
      })
      .catch((cause) => !cancelled && setError((cause as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => { cancelled = true; };
  }, []);


  return {
    clientId,
    clientSecret,
    error,
    loading,
    save,
    saving,
    setClientId,
    setClientSecret,
    settings,
  };
}
