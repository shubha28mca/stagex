// Stepper — the 3-step registration progress indicator. `steps` is a list of
// labels; `current` is the 0-based active index.
export default function Stepper({ steps, current }) {
  return (
    <div className="stepper">
      {steps.map((label, i) => {
        const state = i < current ? "done" : i === current ? "cur" : "";
        return (
          <div className={`stp ${state}`} key={label}>
            <span className="c">{i < current ? "✓" : i + 1}</span>
            <b>{label}</b>
            {i < steps.length - 1 && <span className="bar" />}
          </div>
        );
      })}
    </div>
  );
}
