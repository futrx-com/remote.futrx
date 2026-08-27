// Grid — two-column responsive field grid used inside Panels.
import { Grid, Field } from "remote.futrx-web";

export const TwoColumns = () => (
  <div className="w-full max-w-2xl">
    <Grid>
      <Field label="Memory used" value="1.9 GiB / 8.0 GiB" mono />
      <Field label="Memory peak" value="3.2 GiB" mono />
      <Field label="Swap" value="0 B" mono />
      <Field label="Disk (rootfs)" value="11.4 GiB" mono />
      <Field label="CPU time" value="9,412 s" mono />
      <Field label="Processes" value="24" mono />
    </Grid>
  </div>
);

export const OddCount = () => (
  <div className="w-full max-w-2xl">
    <Grid>
      <Field label="CPU" value="4" mono />
      <Field label="Memory" value="8GiB" mono />
      <Field label="Disk" value="40GiB" mono />
    </Grid>
  </div>
);

export const PairedPaths = () => (
  <div className="w-full max-w-2xl">
    <Grid>
      <Field label="Host source" value="/Users/ahmed/dev/remote.futrx" mono />
      <Field label="Container path" value="/workspace" mono />
    </Grid>
  </div>
);
