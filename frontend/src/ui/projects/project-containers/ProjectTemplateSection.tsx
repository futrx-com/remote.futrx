import type { JSX } from "preact";
import type { ProjectTemplateStatus } from "../../../models/template";
import { Field, Grid, Panel } from "./ProjectContainerPrimitives";
import { formatEpochMillis } from "./projectContainerFormat";

/**
 * Small badge shown next to the project name. Templates that install nothing
 * ("blank") still get a badge so the field is never mysteriously absent.
 */
export function TemplateBadge({ template }: { template: ProjectTemplateStatus }) {
  return (
    <span
      class="inline-flex items-center h-5 px-1.5 rounded bg-white/[0.06] text-[11px] font-medium text-ink-200"
      title={`Project template: ${template.title || template.name}`}
    >
      {template.title || template.name}
    </span>
  );
}

/**
 * Provisioning progress badge. Only rendered for templates that actually
 * install something — "none" means the container was ready on first boot.
 */
export function TemplateStatusBadge({ status }: { status: ProjectTemplateStatus["status"] }) {
  if (status === "none") return null;
  const tone =
    status === "done"
      ? "text-accent-green bg-accent-green/[0.12]"
      : status === "failed"
      ? "text-accent-red bg-accent-red/[0.12]"
      : status === "running"
      ? "text-accent-blue bg-accent-blue/[0.12]"
      : "text-ink-300 bg-white/[0.06]";
  return (
    <span class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${tone}`}>
      {statusLabel(status)}
    </span>
  );
}

export function statusLabel(status: ProjectTemplateStatus["status"]): string {
  switch (status) {
    case "pending":
      return "setup pending";
    case "running":
      return "setting up";
    case "done":
      return "setup done";
    case "failed":
      return "setup failed";
    default:
      return "";
  }
}

export function TemplatePanel({ template }: { template: ProjectTemplateStatus }) {
  const fields: JSX.Element[] = [
    <Field key="template" label="Template" value={template.title || template.name} />,
    <Field
      key="provisioning"
      label="Provisioning"
      value={template.status === "none" ? "not required" : statusLabel(template.status)}
      tone={template.status === "failed" ? "warn" : undefined}
    />,
    <Field key="started" label="Started" value={formatEpochMillis(template.startedAt)} mono />,
    <Field key="finished" label="Finished" value={formatEpochMillis(template.finishedAt)} mono />,
  ];
  if (template.logPath) {
    fields.push(
      <Field key="log" label="Log (in container)" value={template.logPath} mono />
    );
  }

  return (
    <Panel title="Template">
      <div>
        <Grid>{fields}</Grid>
        {template.error && (
          <p class="mt-2 text-[12px] text-accent-red break-words">{template.error}</p>
        )}
        {template.status === "failed" && (
          <p class="mt-2 text-[12px] text-ink-300 leading-snug">
            Restart the project to retry: provisioning re-runs on every container
            convergence until it succeeds.
          </p>
        )}
      </div>
    </Panel>
  );
}
