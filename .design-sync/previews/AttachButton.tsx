// AttachButton — "+" icon button that opens the hidden file input in the composer.
import { AttachButton } from "remote.futrx-web";

const noop = () => {};
const inputRef = { current: null };

export const Default = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <AttachButton
      fileInputRef={inputRef}
      uploading={false}
      disconnected={false}
      onFilesSelected={noop}
    />
  </div>
);

export const Uploading = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <AttachButton
      fileInputRef={inputRef}
      uploading={true}
      disconnected={false}
      onFilesSelected={noop}
    />
  </div>
);

export const Disconnected = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <AttachButton
      fileInputRef={inputRef}
      uploading={false}
      disconnected={true}
      onFilesSelected={noop}
    />
  </div>
);
