// Small reusable primitives for the admin console. They depend only on
// theme.css classes, so the whole folder can be lifted into another project.
import { useMemo, useState } from "react";

// usePaged slices a list into client-side pages for any admin table.
export function usePaged(items, pageSize = 10) {
  const [page, setPage] = useState(1);
  const total = items?.length || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const current = Math.min(page, totalPages);
  const pageItems = useMemo(
    () => (items || []).slice((current - 1) * pageSize, current * pageSize),
    [items, current, pageSize]
  );
  return { pageItems, page: current, setPage, totalPages };
}

// Pagination renders prev/next controls; hidden when there is a single page.
export function Pagination({ page, totalPages, onChange }) {
  if (totalPages <= 1) return null;
  return (
    <div className="pager">
      <button className="btn btn-o btn-sm" disabled={page <= 1} onClick={() => onChange(page - 1)}>← Prev</button>
      <span className="muted" style={{ fontSize: ".82rem" }}>Page {page} of {totalPages}</span>
      <button className="btn btn-o btn-sm" disabled={page >= totalPages} onClick={() => onChange(page + 1)}>Next →</button>
    </div>
  );
}

export function Button({ variant = "p", size, block, className = "", ...props }) {
  const cls = ["btn", `btn-${variant}`, size === "sm" ? "btn-sm" : "", block ? "btn-block" : "", className]
    .filter(Boolean)
    .join(" ");
  return <button className={cls} {...props} />;
}

export function Badge({ tone = "purple", children }) {
  return <span className={`badge b-${tone}`}>{children}</span>;
}

export function Panel({ title, actions, children }) {
  return (
    <section className="panel">
      {(title || actions) && (
        <header className="ph">
          {title && <h3>{title}</h3>}
          {actions}
        </header>
      )}
      <div className="pb">{children}</div>
    </section>
  );
}

export function Field({ as = "input", label, value, onChange, options, ...props }) {
  const handle = (e) => onChange?.(e.target.value);
  return (
    <div className="f">
      {as === "select" ? (
        <select className={value !== "" && value != null ? "filled" : ""} value={value} onChange={handle} {...props}>
          <option value="" disabled hidden></option>
          {options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : as === "textarea" ? (
        <textarea placeholder=" " value={value} onChange={handle} {...props} />
      ) : (
        <input placeholder=" " value={value} onChange={handle} {...props} />
      )}
      <label>{label}</label>
    </div>
  );
}

export function Modal({ title, onClose, children, footer }) {
  return (
    <div className="overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="mh">
          <h3 style={{ color: "var(--navy)" }}>{title}</h3>
          <button className="x" onClick={onClose}>✕</button>
        </div>
        <div className="mb">{children}</div>
        {footer && <div className="mf">{footer}</div>}
      </div>
    </div>
  );
}

export function Spinner() {
  return <div className="spinner" role="status" aria-label="Loading" />;
}
export function Alert({ tone = "err", children }) {
  if (!children) return null;
  return <div className={`alert ${tone}`}>{children}</div>;
}
export function Empty({ children }) {
  return <div className="empty">{children}</div>;
}

export function initials(name = "") {
  return name.split(" ").filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join("");
}
