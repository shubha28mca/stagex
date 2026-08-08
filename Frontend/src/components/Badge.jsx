// Badge — a small status pill. `tone` selects the color scheme from theme.css.
export default function Badge({ tone = "purple", children }) {
  return <span className={`badge b-${tone}`}>{children}</span>;
}
