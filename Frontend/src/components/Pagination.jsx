import { useMemo, useState } from "react";

// usePaged slices a list into pages on the client. Returns the current page's
// items plus controls, so any list screen becomes paginated with one hook.
export function usePaged(items, pageSize = 9) {
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

// Pagination renders prev/next controls; it hides itself when there is one page.
export default function Pagination({ page, totalPages, onChange }) {
  if (totalPages <= 1) return null;
  return (
    <div className="pager">
      <button className="btn btn-o btn-sm" disabled={page <= 1} onClick={() => onChange(page - 1)}>
        ← Prev
      </button>
      <span className="muted" style={{ fontSize: ".82rem" }}>Page {page} of {totalPages}</span>
      <button className="btn btn-o btn-sm" disabled={page >= totalPages} onClick={() => onChange(page + 1)}>
        Next →
      </button>
    </div>
  );
}
