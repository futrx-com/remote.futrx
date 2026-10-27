// Loading — inline placeholder card used while a project section fetches data.
import { Loading } from "remote.futrx-web";

export const FetchingContainer = () => (
  <div className="w-full max-w-xl">
    <Loading text="Loading container data…" />
  </div>
);

export const NoData = () => (
  <div className="w-full max-w-xl">
    <Loading text="No data." />
  </div>
);

export const FetchingSecrets = () => (
  <div className="w-full max-w-xl">
    <Loading text="Loading secrets…" />
  </div>
);
