import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Badge, Button, Empty, Modal, Panel, Spinner } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

// Ticket sales — Operational Admin oversight.
//
// Answers "how many tickets are booked?" per event, and gives Ops the single
// write the participant side cannot do: marking an offline refund as settled.
// Refunds never move money automatically anywhere in this system, so a
// cancelled booking waits in `pending_offline` until someone here records the
// payout.
//
// Ops does not create or edit bookings — those belong to the family that made
// them — so this screen is read-only apart from that one action.

const money = (n) => "₹" + Number(n || 0).toLocaleString("en-IN", { maximumFractionDigits: 0 });
const num = (n) => Number(n || 0).toLocaleString("en-IN");
const fmtDate = (d) =>
  new Date(d).toLocaleDateString("en-IN", { day: "numeric", month: "short", year: "numeric" });

const STATUS_TONE = {
  confirmed: "open",
  cancelled: "draft",
  expired: "err",
  pending_payment: "review",
  payment_processing: "review",
};

export default function TicketSalesPage() {
  const navigate = useNavigate();
  const [stats, setStats] = useState([]);
  const [bookings, setBookings] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [eventFilter, setEventFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

  // The refund being settled, plus its payout reference.
  const [settling, setSettling] = useState(null);
  const [payoutRef, setPayoutRef] = useState("");
  const [busy, setBusy] = useState(false);

  const load = async (eventId = eventFilter, status = statusFilter) => {
    setLoading(true);
    try {
      const [s, b] = await Promise.all([
        opsApi.ticketStats(),
        opsApi.bookings(eventId, status),
      ]);
      setStats(s || []);
      setBookings(b || []);
      setError("");
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, []);

  const applyFilters = (eventId, status) => {
    setEventFilter(eventId);
    setStatusFilter(status);
    load(eventId, status);
  };

  const settle = async () => {
    setBusy(true);
    setError("");
    try {
      await opsApi.settleRefund(settling.id, "completed", payoutRef);
      setSettling(null);
      setPayoutRef("");
      setNotice("Refund marked as paid out.");
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const reopen = async (b) => {
    setBusy(true);
    setError("");
    try {
      await opsApi.settleRefund(b.id, "pending_offline", "");
      setNotice("Refund put back on the pending queue.");
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // Platform-wide rollups across every event that sells reserved seating.
  const totals = stats.reduce(
    (a, r) => ({
      booked: a.booked + r.ticketsBooked,
      cancelled: a.cancelled + r.ticketsCancelled,
      revenue: a.revenue + Number(r.grossRevenue || 0),
      pending: a.pending + Number(r.refundPending || 0),
      held: a.held + r.seatsHeld,
    }),
    { booked: 0, cancelled: 0, revenue: 0, pending: 0, held: 0 }
  );

  if (loading) return <Spinner />;

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>Ticket Sales</h1>
          <p>
            Seat bookings across every event with a published seating
            arrangement. Refunds are settled offline and recorded here.
          </p>
        </div>
        <Button variant="o" onClick={() => load()}>Refresh</Button>
      </div>

      <Alert>{error}</Alert>
      {notice && <Alert tone="ok">{notice}</Alert>}

      <div className="kpis">
        <div className="kpi"><div className="n">{num(totals.booked)}</div><div className="l">TICKETS BOOKED</div></div>
        <div className="kpi"><div className="n">{money(totals.revenue)}</div><div className="l">GROSS REVENUE</div></div>
        <div className="kpi"><div className="n">{num(totals.held)}</div><div className="l">SEATS BEING BOOKED NOW</div></div>
        <div className="kpi"><div className="n">{money(totals.pending)}</div><div className="l">REFUNDS PENDING</div></div>
      </div>

      <Panel title="By event">
        {stats.length === 0 ? (
          <Empty>
            No events sell reserved seating yet — publish a hall's seating
            arrangement under <b>Seat Maps</b> first.
          </Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Event</th>
                <th>Starts</th>
                <th>Booked</th>
                <th>Being booked</th>
                <th>Available</th>
                <th>Blocked</th>
                <th>Cancelled</th>
                <th>Revenue</th>
                <th>Refunds</th>
                <th style={{ textAlign: "right" }}>Bookings</th>
              </tr>
            </thead>
            <tbody>
              {stats.map((r) => (
                <tr key={r.eventId}>
                  <td><b>{r.eventName}</b></td>
                  <td>{fmtDate(r.startDate)}</td>
                  <td>
                    <b>{num(r.ticketsBooked)}</b>
                    <span className="muted"> / {num(r.mappedSeats)}</span>
                  </td>
                  {/* Seats held by a buyer mid-purchase — not yet sold, not
                      available either. */}
                  <td>{r.seatsHeld > 0 ? <Badge tone="review">{r.seatsHeld}</Badge> : "—"}</td>
                  <td><b>{num(r.seatsAvailable)}</b></td>
                  {/* Not sellable, but Ops still has to account for them. */}
                  <td>
                    {r.blockedSeats + r.houseSeats > 0 ? (
                      <span title={`${r.blockedSeats} blocked, ${r.houseSeats} house hold`}>
                        {num(r.blockedSeats + r.houseSeats)}
                      </span>
                    ) : "—"}
                  </td>
                  <td>{r.ticketsCancelled > 0 ? num(r.ticketsCancelled) : "—"}</td>
                  <td>{money(r.grossRevenue)}</td>
                  <td>
                    {Number(r.refundPending) > 0 ? (
                      <Badge tone="err">{money(r.refundPending)} due</Badge>
                    ) : Number(r.refundCompleted) > 0 ? (
                      <span className="muted">{money(r.refundCompleted)} paid</span>
                    ) : "—"}
                  </td>
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      {/* Seat-level occupancy and the reservation report. */}
                      <Button size="sm"
                              onClick={() => navigate(`/ops/events/${r.eventId}/seats`)}>
                        Seats &amp; report
                      </Button>
                      <Button variant="o" size="sm"
                              onClick={() => applyFilters(r.eventId, "")}>
                        {r.confirmedBookings + r.cancelledBookings} bookings
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel
        title="Bookings"
        actions={
          <div className="row" style={{ gap: 8 }}>
            <select
              value={statusFilter}
              onChange={(e) => applyFilters(eventFilter, e.target.value)}
              style={{ border: "1.5px solid var(--line)", borderRadius: 9, padding: "7px 10px", fontSize: ".8rem" }}
            >
              <option value="">All statuses</option>
              <option value="confirmed">Confirmed</option>
              <option value="cancelled">Cancelled</option>
              <option value="pending_payment">Awaiting payment</option>
              <option value="payment_processing">Payment processing</option>
              <option value="expired">Expired</option>
            </select>
            {(eventFilter || statusFilter) && (
              <Button variant="o" size="sm" onClick={() => applyFilters("", "")}>
                Clear filters
              </Button>
            )}
          </div>
        }
      >
        {bookings.length === 0 ? (
          <Empty>No bookings match these filters.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Reference</th>
                <th>Event</th>
                <th>Booked by</th>
                <th>Seats</th>
                <th>Total</th>
                <th>Status</th>
                <th>Refund</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {bookings.map((b) => (
                <tr key={b.id}>
                  <td>
                    <b style={{ fontFamily: "Poppins", fontSize: ".78rem" }}>{b.reference}</b>
                    <br />
                    <span className="muted" style={{ fontSize: ".7rem" }}>
                      {fmtDate(b.createdAt)}
                    </span>
                  </td>
                  <td>{b.eventName}</td>
                  <td>
                    {b.familyName || "—"}
                    <br />
                    <span className="muted" style={{ fontSize: ".72rem" }}>{b.phone}</span>
                  </td>
                  <td>
                    <span title={b.items.map((i) => i.seatLabel).join(", ")}>
                      {b.seatCount}
                    </span>
                    <br />
                    <span className="muted" style={{ fontSize: ".7rem" }}>
                      {b.items.slice(0, 4).map((i) => i.seatLabel).join(", ")}
                      {b.items.length > 4 ? ` +${b.items.length - 4}` : ""}
                    </span>
                  </td>
                  <td>{money(b.total)}</td>
                  <td>
                    <Badge tone={STATUS_TONE[b.status] || "purple"}>{b.status.replace(/_/g, " ")}</Badge>
                    {/* Why it was cancelled — the record the buyer also sees. */}
                    {b.cancellationReason && (
                      <>
                        <br />
                        <span className="muted" style={{ fontSize: ".7rem" }}>
                          {b.cancellationReason.replace(/_/g, " ")}
                          {b.cancelledBy === "system" ? " (auto)" : ""}
                        </span>
                      </>
                    )}
                  </td>
                  <td>
                    {b.refundStatus === "pending_offline" ? (
                      <Badge tone="err">{money(b.refundAmount)} due</Badge>
                    ) : b.refundStatus === "completed" ? (
                      <Badge tone="open">{money(b.refundAmount)} paid</Badge>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      {b.refundStatus === "pending_offline" && (
                        <Button size="sm" disabled={busy}
                                onClick={() => { setSettling(b); setPayoutRef(""); }}>
                          Mark refunded
                        </Button>
                      )}
                      {b.refundStatus === "completed" && (
                        <Button variant="o" size="sm" disabled={busy} onClick={() => reopen(b)}>
                          Undo
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      {settling && (
        <Modal
          title={`Record refund for ${settling.reference}`}
          onClose={() => setSettling(null)}
          footer={
            <>
              <Button variant="o" onClick={() => setSettling(null)}>Cancel</Button>
              <Button onClick={settle} disabled={busy}>
                {busy ? "Saving…" : `Mark ${money(settling.refundAmount)} as paid`}
              </Button>
            </>
          }
        >
          <p className="muted" style={{ fontSize: ".84rem", marginBottom: 14 }}>
            Confirm that <b>{money(settling.refundAmount)}</b> has been returned to{" "}
            <b>{settling.familyName || settling.phone}</b> outside the platform.
            This only records the payout — it does not move any money.
          </p>
          <div className="f">
            <input placeholder=" " value={payoutRef}
                   onChange={(e) => setPayoutRef(e.target.value)} />
            <label>Payout reference (bank UTR, receipt no.)</label>
          </div>
        </Modal>
      )}
    </>
  );
}
