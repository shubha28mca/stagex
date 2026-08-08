// StarRating — 1..5 clickable stars (Feedback screen). Controlled via `value`.
export default function StarRating({ value = 0, onChange }) {
  return (
    <div className="stars" role="radiogroup">
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          key={n}
          type="button"
          className={`star ${n <= value ? "on" : ""}`}
          aria-label={`${n} star${n > 1 ? "s" : ""}`}
          aria-checked={n === value}
          role="radio"
          onClick={() => onChange?.(n)}
        >
          ★
        </button>
      ))}
    </div>
  );
}
