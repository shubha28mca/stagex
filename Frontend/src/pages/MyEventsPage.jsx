import { useEffect, useState } from "react";
import { myApi } from "../api/endpoints";
import { Panel, Badge, Spinner, Alert, Empty, Pagination, usePaged } from "../components";

const regTone = { paid: "open", pending: "pending", held: "pending", cancelled: "done" };
const posTone = { gold: "gold", silver: "purple", bronze: "pending", participation: "done" };

// MyEventsPage — the family's registrations with per-participant entries, plus
// the Event Admin's notifications, media, winners and certificates (§6).
export default function MyEventsPage() {
  const [items, setItems] = useState([]);
  const [notes, setNotes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const { pageItems, page, setPage, totalPages } = usePaged(items, 6);

  useEffect(() => {
    (async () => {
      try {
        const [events, notifications] = await Promise.all([myApi.events(), myApi.notifications()]);
        setItems(events || []);
        setNotes(notifications || []);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  if (loading) return <Spinner />;

  return (
    <>
      <div className="mb">
        <h1 style={{ color: "var(--navy)", fontSize: "1.45rem" }}>My Events</h1>
        <p className="muted">Everything your family has registered for.</p>
      </div>
      <Alert>{error}</Alert>

      {notes.length > 0 && (
        <Panel title="🔔 Notifications">
          {notes.map((n, i) => (
            <div key={i} style={{ padding: "8px 0", borderBottom: "1px solid var(--line)" }}>
              <div className="row spread">
                <b style={{ fontSize: ".9rem" }}>{n.title}</b>
                <span className="muted" style={{ fontSize: ".72rem" }}>{n.eventName} · {n.createdAt}</span>
              </div>
              <div className="muted" style={{ fontSize: ".82rem" }}>{n.message}</div>
            </div>
          ))}
        </Panel>
      )}

      {items.length === 0 ? (
        <Empty>No registrations yet — discover an event to get started.</Empty>
      ) : (
        <>
          {pageItems.map((r) => (
            <Panel
              key={r.registrationId}
              title={r.eventName}
              actions={<Badge tone={regTone[r.status] || "purple"}>{r.status}</Badge>}
            >
              <div className="muted mb" style={{ fontSize: ".82rem" }}>
                📍 {r.city} · {new Date(r.startDate).toLocaleDateString()} · Paid ₹{r.total}
              </div>
              {r.entries.map((en) => (
                <div className="row spread" key={en.entryId} style={{ padding: "6px 0", borderBottom: "1px solid var(--line)" }}>
                  <span>{en.personName} · {en.categoryName}</span>
                  <Badge tone="purple">#{en.entryCode}</Badge>
                </div>
              ))}

              {r.winners?.length > 0 && (
                <div className="mt">
                  <b style={{ fontSize: ".85rem", color: "var(--navy)" }}>🏆 Winners</b>
                  <div className="row" style={{ gap: 8, marginTop: 6 }}>
                    {r.winners.map((w, i) => (
                      <Badge key={i} tone={posTone[w.position] || "purple"}>{w.position} · {w.personName}</Badge>
                    ))}
                  </div>
                </div>
              )}

              {r.certificates?.length > 0 && (
                <div className="mt">
                  <b style={{ fontSize: ".85rem", color: "var(--navy)" }}>🎖 Your certificates</b>
                  {r.certificates.map((c, i) => (
                    <div className="row spread" key={i} style={{ padding: "6px 0" }}>
                      <span>{c.personName} · {c.position} · #{c.certCode}</span>
                      {c.fileUrl && (
                        <a className="btn btn-o btn-sm" href={c.fileUrl} target="_blank" rel="noreferrer">View certificate</a>
                      )}
                    </div>
                  ))}
                </div>
              )}

              {r.media?.length > 0 && (
                <div className="mt">
                  <b style={{ fontSize: ".85rem", color: "var(--navy)" }}>📸 Event gallery</b>
                  <div className="row" style={{ gap: 10, flexWrap: "wrap", marginTop: 6 }}>
                    {r.media.map((m, i) =>
                      m.kind === "video" ? (
                        <video key={i} src={m.url} controls style={{ width: 160, height: 110, borderRadius: 10, objectFit: "cover", background: "#000" }} />
                      ) : (
                        <a key={i} href={m.url} target="_blank" rel="noreferrer">
                          <img src={m.url} alt="" style={{ width: 160, height: 110, borderRadius: 10, objectFit: "cover" }} />
                        </a>
                      )
                    )}
                  </div>
                </div>
              )}
            </Panel>
          ))}
          <Pagination page={page} totalPages={totalPages} onChange={setPage} />
        </>
      )}
    </>
  );
}
