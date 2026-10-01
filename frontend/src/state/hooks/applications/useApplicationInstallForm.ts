import { useState } from "preact/hooks";
import type { AppApplication } from "../../../models/application";
import type { ApplicationsController } from "./useApplications";
import { prepareInstallRequest } from "../../../services/applications/prepareInstallRequest";

export function useApplicationInstallForm(application: AppApplication, asksForPort: boolean, onInstall: ApplicationsController["install"], onClose: () => void) {
  const [name, setName] = useState(application.name);
  const [env, setEnv] = useState<Record<string, string>>({});
  const [externalPort, setExternalPort] = useState<string>("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (event: Event) => {
    event.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await onInstall(prepareInstallRequest(application, { name, env, externalPort }, asksForPort));
      onClose();
    } catch (error) {
      setErr((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return { name, setName, env, setEnv, externalPort, setExternalPort, busy, err, submit };
}
