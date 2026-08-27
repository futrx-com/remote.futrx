// Panel — titled section frame for project container pages; children are
// usually a Grid of Fields (see ProjectInfoSection).
import { Panel, Grid, Field, Loading, Empty } from "remote.futrx-web";

export const Overview = () => (
  <div className="w-full max-w-2xl">
    <Panel title="Overview">
      <Grid>
        <Field label="Container" value="futrx-web-prod" mono />
        <Field label="State" value="RUNNING" mono />
        <Field label="PID" value="41837" mono />
        <Field label="Processes" value="24" mono />
        <Field label="Image" value="ubuntu/24.04 (release)" />
        <Field label="Architecture" value="aarch64" mono />
        <Field label="Boot autostart" value="no" mono />
        <Field label="Created" value="2026-05-02 14:11" mono />
        <Field label="Last used" value="2026-08-16 09:32" mono />
      </Grid>
    </Panel>
  </div>
);

export const ResourceLimits = () => (
  <div className="w-full max-w-2xl">
    <Panel title="Resource limits">
      <Grid>
        <Field label="CPU" value="4" mono />
        <Field label="Memory" value="8GiB" mono />
        <Field label="Disk" value="40GiB" mono />
      </Grid>
    </Panel>
  </div>
);

export const LoadingChild = () => (
  <div className="w-full max-w-2xl">
    <Panel title="Network">
      <Loading text="Loading container data…" />
    </Panel>
  </div>
);

export const EmptyChild = () => (
  <div className="w-full max-w-2xl">
    <Panel title="Secrets">
      <Empty text="No secrets yet." compact />
    </Panel>
  </div>
);
