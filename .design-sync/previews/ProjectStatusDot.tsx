// ProjectStatusDot — tiny container-state dot shown next to project entries.
import { ProjectStatusDot } from "remote.futrx-web";

const Row = ({ status, label }: { status: any; label: string }) => (
  <div className="flex items-center gap-2">
    <ProjectStatusDot status={status} />
    <span className="text-[12px] text-ink-300">{label}</span>
  </div>
);

export const AllStatuses = () => (
  <div className="w-full flex flex-col gap-2" style={{ maxWidth: "320px" }}>
    <Row status="running" label="Running" />
    <Row status="provisioning" label="Provisioning" />
    <Row status="stopped" label="Stopped" />
    <Row status="error" label="Error" />
    <Row status="missing" label="Missing - needs reprovision" />
    <Row status="" label="Unknown" />
  </div>
);

export const Running = () => (
  <div className="w-full flex items-center gap-2" style={{ maxWidth: "320px" }}>
    <ProjectStatusDot status="running" />
    <span className="text-[13px] text-ink-100">remote.futrx</span>
  </div>
);

export const Provisioning = () => (
  <div className="w-full flex items-center gap-2" style={{ maxWidth: "320px" }}>
    <ProjectStatusDot status="provisioning" />
    <span className="text-[13px] text-ink-100">docs-site</span>
  </div>
);

export const Missing = () => (
  <div className="w-full flex items-center gap-2" style={{ maxWidth: "320px" }}>
    <ProjectStatusDot status="missing" />
    <span className="text-[13px] text-ink-100">legacy-importer</span>
  </div>
);
