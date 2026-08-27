// LoadingScreen — full-app boot spinner shown by AuthGate while auth resolves.
// Uses .app-shell (position: fixed), so each story pins it inside a sized frame
// via a transform containing block + --app-height: 100%.
import { LoadingScreen } from "remote.futrx-web";

const frame = (height: number) =>
  ({
    transform: "translateZ(0)",
    height,
    "--app-height": "100%",
  }) as React.CSSProperties;

export const Default = () => (
  <div className="relative w-full max-w-xl overflow-hidden rounded-lg border border-white/[0.08]" style={frame(280)}>
    <LoadingScreen />
  </div>
);

export const ShortViewport = () => (
  <div className="relative w-full max-w-md overflow-hidden rounded-lg border border-white/[0.08]" style={frame(160)}>
    <LoadingScreen />
  </div>
);
