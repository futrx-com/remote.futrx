// SendControls — send/queue button plus cancel button while streaming.
import { SendControls } from "remote.futrx-web";

const noop = () => {};

export const ReadyToSend = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <SendControls streaming={false} canSend={true} disconnected={false} onCancel={noop} />
  </div>
);

export const DisabledEmpty = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <SendControls streaming={false} canSend={false} disconnected={false} onCancel={noop} />
  </div>
);

export const StreamingQueue = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <SendControls streaming={true} canSend={true} disconnected={false} onCancel={noop} />
  </div>
);

export const Disconnected = () => (
  <div className="w-full max-w-md flex items-center gap-2">
    <SendControls streaming={false} canSend={false} disconnected={true} onCancel={noop} />
  </div>
);
