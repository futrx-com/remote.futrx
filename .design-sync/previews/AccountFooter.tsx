// AccountFooter — signed-in account row pinned to the bottom of the sidebar.
import { AccountFooter } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <AccountFooter email="me@ahmedwaleed.net" onOpenSettings={() => {}} />
  </div>
);

export const NoSettingsButton = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <AccountFooter email="me@ahmedwaleed.net" />
  </div>
);

export const LongEmailTruncates = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <AccountFooter
      email="ahmed.waleed.mohamed.engineering@very-long-company-domain.example.com"
      onOpenSettings={() => {}}
    />
  </div>
);
