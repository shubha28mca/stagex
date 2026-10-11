import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Badge, Button, Empty, Panel, Spinner } from "../components";
import { myApi, ticketsApi } from "../api/endpoints";

// =============================================================================
// My Bookings — the buyer's ticket history and the cancellation flow.
//
// Bookings are never removed from this list. A cancelled booking keeps its
// seats, its price, the reason it was cancelled and what is owed back, because
// that record is the buyer's only proof of what happened.
//
// Two rules are surfaced rather than hidden:
//
//   * cancellation closes once the event starts. The server decides
//     (`canCancel` / `cannotCancelWhy`) and this page only renders that
//     decision, so the button state can never disagree with the API.
//   * refunds are handled OFFLINE. Nothing here promises an automatic reversal;
//     the amount owed is shown with its settlement state.
// =============================================================================

const money = (n) => "₹" + Number(n || 0).toLocaleString("en-IN", { maximumFractionDigits: 0 });

const fmtDate = (d) =>
  new Date(d).toLocaleDateString("en-IN", { day: "numeric", month: "short", year: "numeric" });

const fmtDateTime = (d) =>
  new Date(d).toLocaleString("en-IN", {
    day: "numeric", month: "short", year: "numeric", hour: "numeric", minute: "2-digit",
  });

// Status pills map onto the theme's existing badge tones.
const STATUS = {
  confirmed: { tone: "open", label: "confirmed" },
  cancelled: { tone: "done", label: "cancelled" },
  expired: { tone: "err", label: "expired" },
  pending_payment: { tone: "pending", label: "awaiting payment" },
  payment_processing: { tone: "gold", label: "payment processing" },
};

const REFUND = {
  pending_offline: { tone: "pending", label: "refund pending (offline)" },
  completed: { tone: "open", label: "refund paid" },
};

export default function MyBookingsPage() {
  const navigate = useNavigate();
  const [bookings, setBookings] = useState([]);
  const [reasons, setReasons] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // The booking being cancelled, plus the form for it.
  const [cancelling, setCancelling] = useState(null);
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const [list, opts] = await Promise.all([
        myApi.bookings(),
        // The reason list comes from the server so this page never keeps its
        // own copy of a set the API validates against.
        ticketsApi.cancellationReasons(),
      ]);
      setBookings(list || []);
      setReasons(opts || []);
      setError("");
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const openCancel = (b) => {
    setCancelling(b);
    setReason("");
    setNote("");
    setError("");
    setNotice("");
  };

  const submitCancel = async () => {
    setBusy(true);
    setError("");
    try {
      const res = await myApi.cancelBooking(cancelling.id, reason, note);
      setCancelling(null);
      // Say exactly what will happen with the money, using the server's own
      // wording so there is one description of the offline process.
      setNotice(res.refund?.message || "Your booking has been cancelled.");
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Spinner />;

  const active = bookings.filter((b) => b.status === "confirmed");
  const inProgress = bookings.filter(
    (b) => b.status === "pending_payment" || b.status === "payment_processing");
  const past = bookings.filter(
    (b) => b.status === "cancelled" || b.status === "expired");

  return (
    <>
      <div className="spread row" style={{ marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: "1.5rem", color: "var(--ink)" }}>My Bookings</h1>
          <p className="muted" style={{ fontSize: ".85rem" }}>
            Your ticket bookings, including cancelled ones.
          </p>
        </div>
        <Button variant="o" onClick={() => navigate("/discover")}>Browse events</Button>
      </div>

      {error && <div className="alert err">{error}</div>}
      {notice && <div className="alert ok">{notice}</div>}

      {bookings.length === 0 && (
        <Empty>
          No bookings yet. Find an event on <b>Discover</b> and book your seats.
        </Empty>
      )}

      {inProgress.length > 0 && (
        <Section title="Awaiting payment" bookings={inProgress}
                 onCancel={openCancel} navigate={navigate} />
      )}
      {active.length > 0 && (
        <Section title="Your tickets" bookings={active}
                 onCancel={openCancel} navigate={navigate} />
      )}
      {past.length > 0 && (
        <Section title="Cancelled & expired" bookings={past}
                 onCancel={openCancel} navigate={navigate} />
      )}

      {cancelling && (
        <div className="alert" style={{ background: "#fff", border: "1.5px solid var(--line)" }}>
          <h3 style={{ fontSize: "1rem", marginBottom: 6 }}>
            Cancel booking {cancelling.reference}?
          </h3>
          <p className="muted" style={{ fontSize: ".8rem", marginBottom: 12 }}>
            {cancelling.seatCount} seat{cancelling.seatCount === 1 ? "" : "s"} for{" "}
            <b>{cancelling.eventName}</b> · {money(cancelling.total)} paid.
            <br />
            Your seats go back on sale immediately. Any refund is processed{" "}
            <b>offline by our team</b> — it is not returned automatically to your
            payment method.
          </p>

          <div className="f">
            <select className={reason ? "filled" : ""} value={reason}
                    onChange={(e) => setReason(e.target.value)}>
              <option value="" disabled hidden></option>
              {reasons.map((r) => (
                <option key={r.code} value={r.code}>{r.label}</option>
              ))}
            </select>
            <label>Why are you cancelling?</label>
          </div>

          <div className="f mt">
            <textarea placeholder=" " rows={3} value={note}
                      onChange={(e) => setNote(e.target.value)} />
            <label>
              {reason === "other" ? "Please describe (required)" : "Anything to add? (optional)"}
            </label>
          </div>

          <div className="row mt" style={{ gap: 8 }}>
            <Button onClick={submitCancel}
                    disabled={busy || !reason || (reason === "other" && !note.trim())}>
              {busy ? "Cancelling…" : "Confirm cancellation"}
            </Button>
            <Button variant="o" onClick={() => setCancelling(null)} disabled={busy}>
              Keep my tickets
            </Button>
          </div>
        </div>
      )}
    </>
  );
}

// Section renders one group of bookings under a heading.
function Section({ title, bookings, onCancel, navigate }) {
  return (
    <>
      <h3 style={{ fontSize: ".95rem", color: "var(--muted)", margin: "18px 0 10px" }}>
        {title}
      </h3>
      {bookings.map((b) => (
        <BookingCard key={b.id} b={b} onCancel={onCancel} navigate={navigate} />
      ))}
    </>
  );
}

function BookingCard({ b, onCancel, navigate }) {
  const status = STATUS[b.status] || { tone: "purple", label: b.status };
  const refund = REFUND[b.refundStatus];
  const isCancelled = b.status === "cancelled";

  return (
    <div className={`bkcard ${isCancelled ? "cancelled" : ""}`}>
      <div className="bkhead">
        <div>
          <div className="bkref">{b.reference}</div>
          <h3 style={{ fontSize: "1.05rem", color: "var(--ink)" }}>{b.eventName}</h3>
          <p className="muted" style={{ fontSize: ".78rem" }}>
            {b.hallName ? `${b.hallName} · ` : ""}{b.eventCity} · {fmtDate(b.eventStart)}
          </p>
        </div>
        <div className="row" style={{ gap: 6 }}>
          <Badge tone={status.tone}>{status.label}</Badge>
          {refund && <Badge tone={refund.tone}>{refund.label}</Badge>}
        </div>
      </div>

      {/* Seats stay listed after cancellation — struck through, not removed. */}
      <div className="bkseats">
        {b.items?.map((it) => (
          <span key={it.id} className={`bkchip ${it.status === "cancelled" ? "struck" : ""}`}
                title={`${it.section} · row ${it.rowLabel} · ${it.zoneName || "General"} · ${money(it.price)}`}>
            {it.seatLabel}
          </span>
        ))}
      </div>

      <div className="bkgrid">
        <div>
          <div className="muted" style={{ fontSize: ".68rem" }}>SEATS</div>
          <b style={{ fontFamily: "Poppins" }}>{b.seatCount}</b>
        </div>
        <div>
          <div className="muted" style={{ fontSize: ".68rem" }}>TOTAL</div>
          <b style={{ fontFamily: "Poppins" }}>{money(b.total)}</b>
        </div>
        {b.discount > 0 && (
          <div>
            <div className="muted" style={{ fontSize: ".68rem" }}>DISCOUNT</div>
            <b style={{ fontFamily: "Poppins" }}>− {money(b.discount)}</b>
          </div>
        )}
        <div>
          <div className="muted" style={{ fontSize: ".68rem" }}>BOOKED</div>
          <b style={{ fontFamily: "Poppins", fontSize: ".8rem" }}>{fmtDate(b.createdAt)}</b>
        </div>
      </div>

      {/* The cancellation record — the "historic information" kept for the buyer. */}
      {isCancelled && (
        <div className="bknote">
          <b>Cancelled {b.cancelledAt ? fmtDateTime(b.cancelledAt) : ""}</b>
          {b.cancellationReasonLabel ? ` · ${b.cancellationReasonLabel}` : ""}
          {b.cancelledBy === "system" ? " · cancelled automatically" : ""}
          {b.cancellationNote ? <><br />“{b.cancellationNote}”</> : null}
          {b.refundAmount > 0 && (
            <>
              <br />
              Refund of <b>{money(b.refundAmount)}</b>{" "}
              {b.refundStatus === "completed"
                ? "has been paid out."
                : "is being processed offline by our team."}
              {b.refundNote ? <><br />Reference: {b.refundNote}</> : null}
            </>
          )}
        </div>
      )}

      {b.status === "confirmed" && (
        <div className="row mt" style={{ gap: 8 }}>
          {b.canCancel ? (
            <Button variant="o" size="sm" onClick={() => onCancel(b)}>
              Cancel booking
            </Button>
          ) : (
            // The server's own explanation, so the buyer is never left guessing
            // why the button is gone.
            <span className="muted" style={{ fontSize: ".76rem" }}>
              {b.cannotCancelWhy}
            </span>
          )}
        </div>
      )}

      {(b.status === "pending_payment" || b.status === "payment_processing") && (
        <div className="row mt" style={{ gap: 8 }}>
          <Button size="sm" onClick={() => navigate(`/events/${b.eventId}/book`)}>
            Complete payment
          </Button>
          <span className="muted" style={{ fontSize: ".74rem" }}>
            Seats are held until {fmtDateTime(b.holdExpiresAt)}.
          </span>
        </div>
      )}
    </div>
  );
}
