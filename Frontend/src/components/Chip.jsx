// Chip — a toggleable filter pill (Discover screen). Controlled via `active`.
export default function Chip({ active, onClick, children }) {
  return (
    <button className={`chip ${active ? "on" : ""}`} onClick={onClick} type="button">
      {children}
    </button>
  );
}
