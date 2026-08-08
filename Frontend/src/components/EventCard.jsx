import Badge from "./Badge";

// EventCard — a minimal-info Discover card (ClientDesignWeb §4). Clicking it
// navigates to the event detail via the `onOpen` callback.
const statusTone = { open: "open", live: "live", completed: "done", draft: "pending" };

export default function EventCard({ event, onOpen }) {
  const pct = event.slotsTotal ? Math.round((event.slotsFilled / event.slotsTotal) * 100) : 0;
  const dates = new Date(event.startDate).toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
  });
  return (
    <article className="ecard">
      <div className={`cov grad-${event.coverGradient || "purple"}`}>
        <Badge tone={statusTone[event.status] || "purple"}>{event.status}</Badge>
      </div>
      <div className="bd">
        <span className="tag">{event.eventType || event.mode}</span>
        <h3 style={{ cursor: "pointer" }} onClick={() => onOpen(event.id)}>{event.name}</h3>
        <div className="meta">
          <span>📍 {event.city} · {dates}</span>
          <span>🎭 {event.rounds} round{event.rounds > 1 ? "s" : ""} · {event.mode}</span>
        </div>
        <div className="prog">
          <i style={{ width: `${pct}%` }} />
        </div>
        <div className="row spread mt">
          <div>
            <b style={{ fontFamily: "Montserrat", color: "var(--navy)" }}>
              {event.fee > 0 ? `₹${event.fee}` : "Free"}
            </b>
            <small className="muted"> · {event.slotsFilled}/{event.slotsTotal} filled</small>
          </div>
          <div className="row" style={{ gap: 10 }}>
            <a
              onClick={() => onOpen(event.id)}
              style={{ color: "var(--purple)", fontWeight: 600, fontSize: ".8rem", cursor: "pointer" }}
            >
              View details
            </a>
            <button className="btn btn-p btn-sm" onClick={() => onOpen(event.id)}>
              Register →
            </button>
          </div>
        </div>
      </div>
    </article>
  );
}
