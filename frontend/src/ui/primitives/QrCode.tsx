import { useEffect, useRef } from "preact/hooks";
import { QRGenerator } from "../../services/projects/QRGenerator.js";

const qrGenerator = new QRGenerator();

export function QrCode({ value, size = 200, class: className }: {
  value: string;
  size?: number;
  class?: string;
}) {
  const imgRef = useRef<HTMLImageElement>(null);

  useEffect(() => {
    let cancelled = false;
    qrGenerator.createDataUrl(value, size)
      .then((dataUrl) => {
        if (!cancelled && imgRef.current) imgRef.current.src = dataUrl;
      })
      .catch(() => {
        // Rendering failure just leaves the QR image blank; the enrollment
        // UI always shows the secret/otpauth URI as a manual-entry fallback.
      });
    return () => {
      cancelled = true;
    };
  }, [value, size]);

  return (
    <img
      ref={imgRef}
      width={size}
      height={size}
      alt="Two-factor authentication QR code"
      class={className}
    />
  );
}
