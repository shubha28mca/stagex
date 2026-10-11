import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Badge, Button, Empty, Panel, Spinner } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

// Seat Maps landing page — every hall and the state of its seating
// arrangement, with the one action that matters per row: design it, or create
// it if the hall has none. A hall can have only one arrangement, so this is a
// one-to-one list rather than a nested resource.

export default function SeatMapsPage() {
  const navigate = useNavigate();
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const load = async () => {
    setLoading(true);
    try {
      setRows((await opsApi.seatMaps.index()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { load(); }, []);

  // Publishing from here re-runs the server's completeness gate, so a hall with
  // unpriced seats is refused with a message rather than going live half-done.
  const toggle = async (row) => {
    setBusy(row.hallId);
    setError("");
    try {
      if (row.status === "published") await opsApi.seatMaps.unpublish(row.hallId);
      else await opsApi.seatMaps.publish(row.hallId);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  };

  const remove = async (row) => {
    if (!window.confirm(
      `Delete the seating arrangement for ${row.hallName}?\n\n` +
      `All ${row.totalSeats} seats, their prices and the layout will be removed.`
    )) return;
    setBusy(row.hallId);
    setError("");
    try {
      await opsApi.seatMaps.remove(row.hallId);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  };

  const withMap = rows.filter((r) => r.hasMap);
  const totalSeats = withMap.reduce((a, r) => a + r.totalSeats, 0);

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>Seat Maps</h1>
          <p>
            Design a hall's seating arrangement once, then price seats
            individually or by tier. Published arrangements become the seating
            reference for every event held in that hall.
          </p>
        </div>
      </div>

      <Alert>{error}</Alert>

      <div className="kpis">
        <div className="kpi"><div className="n">{rows.length}</div><div className="l">HALLS</div></div>
        <div className="kpi"><div className="n">{withMap.length}</div><div className="l">WITH AN ARRANGEMENT</div></div>
        <div className="kpi">
          <div className="n">{withMap.filter((r) => r.status === "published").length}</div>
          <div className="l">PUBLISHED</div>
        </div>
        <div className="kpi"><div className="n">{totalSeats.toLocaleString("en-IN")}</div><div className="l">MAPPED SEATS</div></div>
      </div>

      <Panel>
        {loading ? (
          <Spinner />
        ) : rows.length === 0 ? (
          <Empty>No halls yet — add one under Master data → Halls first.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Hall</th>
                <th>City</th>
                <th>Listed capacity</th>
                <th>Arrangement</th>
                <th>Mapped seats</th>
                <th>Tiers</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.hallId}>
                  <td><b>{r.hallName}</b></td>
                  <td>{r.city}</td>
                  <td>{r.capacity}</td>
                  <td>
                    {!r.hasMap ? (
                      <Badge tone="err">none</Badge>
                    ) : r.status === "published" ? (
                      <Badge tone="open">published</Badge>
                    ) : (
                      <Badge tone="draft">draft</Badge>
                    )}
                    {r.hasMap && r.mapName ? <span className="muted"> {r.mapName}</span> : null}
                  </td>
                  <td>
                    {r.hasMap ? r.totalSeats.toLocaleString("en-IN") : "—"}
                    {/* Flag a mapped hall whose seats overrun its own registry
                        capacity — usually a sign the capacity field is stale. */}
                    {r.hasMap && r.capacity > 0 && r.totalSeats > r.capacity && (
                      <span className="muted" title="More mapped seats than the hall's listed capacity">
                        {" "}⚠
                      </span>
                    )}
                  </td>
                  <td>{r.hasMap ? r.zones : "—"}</td>
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      <Button size="sm" onClick={() => navigate(`/ops/halls/${r.hallId}/seat-map`)}>
                        {r.hasMap ? "Design" : "Create"}
                      </Button>
                      {r.hasMap && (
                        <>
                          <Button variant="o" size="sm" disabled={busy === r.hallId}
                                  onClick={() => toggle(r)}>
                            {r.status === "published" ? "Unpublish" : "Publish"}
                          </Button>
                          <Button variant="danger" size="sm" disabled={busy === r.hallId}
                                  onClick={() => remove(r)}>
                            Delete
                          </Button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </>
  );
}
