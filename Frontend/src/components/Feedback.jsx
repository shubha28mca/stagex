// Spinner — a centered loading indicator.
export function Spinner() {
  return <div className="spinner" role="status" aria-label="Loading" />;
}

// Alert — an inline success/error message banner.
export function Alert({ tone = "err", children }) {
  if (!children) return null;
  return <div className={`alert ${tone}`}>{children}</div>;
}

// Empty — a friendly empty-state block.
export function Empty({ children }) {
  return <div className="empty">{children}</div>;
}
