import { useEffect, useState } from "preact/hooks";

const labels = ["Thinking...", "Creating...", "Writing...", "Working on it..."];
const spokes = Array.from({ length: 8 }, (_, index) => index);

export function TurnActivity() {
  const [label, setLabel] = useState(() => labels[Math.floor(Math.random() * labels.length)]);
  const [changing, setChanging] = useState(false);

  useEffect(() => {
    let changeTimer: number | undefined;
    const rotation = window.setInterval(() => {
      setChanging(true);
      changeTimer = window.setTimeout(() => {
        setLabel((current) => {
          const index = labels.indexOf(current);
          const next = (index + 1 + Math.floor(Math.random() * (labels.length - 1))) % labels.length;
          return labels[next];
        });
        setChanging(false);
      }, 180);
    }, 5_000);
    return () => {
      window.clearInterval(rotation);
      if (changeTimer !== undefined) window.clearTimeout(changeTimer);
    };
  }, []);

  return (
    <div class="turn-activity" role="status">
      <svg class="turn-activity-icon" viewBox="0 0 24 24" aria-hidden="true">
        {spokes.map((index) => (
          <line
            key={index}
            x1="12" y1="2" x2="12" y2="6"
            transform={`rotate(${index * 45} 12 12)`}
            style={{ animationDelay: `${-index * 150}ms` }}
          />
        ))}
      </svg>
      <span class={`turn-activity-label ${changing ? "turn-activity-label-changing" : ""}`}>{label}</span>
    </div>
  );
}
