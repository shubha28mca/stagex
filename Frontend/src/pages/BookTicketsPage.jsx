import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button, Panel, Stepper } from "../components";
import { ticketsApi } from "../api/endpoints";

// =============================================================================
// Book tickets — the buyer's seat-selection and payment journey.
//
//   1. Seats     pick from the hall plan; the server holds them exclusively
//   2. Review    coupon + totals, with the hold's countdown running
//   3. Pay       mock gateway, then confirm
//
// THE HOLD IS THE WHOLE DESIGN. Selecting seats calls the API, which grants an
// exclusive claim with a server-set deadline. Two things follow, and both are
// handled deliberately here rather than treated as errors:
//
//   * someone else may take a seat between our last refresh and our click, so
//     a 409 is a NORMAL outcome — we refresh the plan and show which seats went;
//   * the hold expires. The countdown below is only a display of the server's
//     deadline; the server is what enforces it, and when it lapses we send the
//     buyer back to step 1 instead of letting them pay for seats they lost.
//
// The plan is re-fetched on a timer so a buyer staring at the screen sees other
// people's seats grey out live.
// =============================================================================

// Seats are drawn at their stored canvas coordinates; posX/posY are centres.
const SEAT = 20;
const HALF = SEAT / 2;

// How often the plan is re-read while picking, so rival holds appear.
const REFRESH_MS = 10000;

const money = (n) => "₹" + Number(n || 0).toLocaleString("en-IN", { maximumFractionDigits: 0 });

// mmss renders the remaining hold time.
const mmss = (ms) => {
  const s = Math.max(0, Math.floor(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
};

// Seat fill by availability. Only `available` uses the tier colour — a seat you
// cannot have should not look enticing.
function seatFill(seat, selected) {
  if (selected) return "#0e1237";
  switch (seat.availability) {
    case "booked": return "#d9d6e3";
    case "held": return "#f3b0b3";
    case "held_by_you": return "#0e1237";
    default: return seat.zoneColor || "#c9c4d8";
  }
}

export default function BookTicketsPage() {
  const { id: eventId } = useParams();
  const navigate = useNavigate();

  const [map, setMap] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const [step, setStep] = useState(0);
  const [picked, setPicked] = useState(() => new Set()); // seatIds chosen locally
  const [hold, setHold] = useState(null);
  const [coupon, setCoupon] = useState("");
  const [quote, setQuote] = useState(null);
  const [booking, setBooking] = useState(null);
  const [intent, setIntent] = useState(null);
  const [remaining, setRemaining] = useState(0);

  // Seats the buyer just lost to someone else, highlighted on the plan.
  const [lost, setLost] = useState([]);

  const zoomRef = useRef(1);

  // ---- load the plan -----------------------------------------------------
  const load = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const m = await ticketsApi.seatMap(eventId);
      setMap(m);
      // Resume an existing hold after a reload, so a refresh mid-checkout does
      // not silently abandon seats the buyer still owns.
      if (m.holdGroupId && m.holdExpiresAt) {
        setHold((h) => h || { holdGroupId: m.holdGroupId, expiresAt: m.holdExpiresAt });
        setPicked((p) => (p.size ? p : new Set(
          m.seats.filter((s) => s.availability === "held_by_you").map((s) => s.seatId))));
      }
      if (!quiet) setError("");
    } catch (e) {
      setError(e.message);
    } finally {
      if (!quiet) setLoading(false);
    }
  }, [eventId]);

  useEffect(() => { load(); }, [load]);

  // Keep the plan fresh while the buyer is still choosing, so rival holds show
  // up without a manual refresh. Stops once we leave the picker.
  useEffect(() => {
    if (step !== 0) return;
    const t = setInterval(() => load(true), REFRESH_MS);
    return () => clearInterval(t);
  }, [step, load]);

  // ---- the countdown ----------------------------------------------------
  // Purely a display of the server's deadline. When it hits zero we do not
  // assume anything — we reload, and the server tells us what is still ours.
  useEffect(() => {
    if (!hold?.expiresAt) return;
    const tick = () => {
      const left = new Date(hold.expiresAt).getTime() - Date.now();
      setRemaining(left);
      if (left <= 0) {
        setHold(null);
        setPicked(new Set());
        setQuote(null);
        setBooking(null);
        setIntent(null);
        setStep(0);
        setError("Your seat hold expired, so the seats went back on sale. Please pick again.");
        load(true);
      }
    };
    tick();
    const t = setInterval(tick, 1000);
    return () => clearInterval(t);
  }, [hold, load]);

  const seatById = useMemo(
    () => new Map((map?.seats || []).map((s) => [s.seatId, s])), [map]);

  const pickedSeats = useMemo(
    () => [...picked].map((id) => seatById.get(id)).filter(Boolean), [picked, seatById]);

  const pickedTotal = useMemo(
    () => pickedSeats.reduce((a, s) => a + s.price, 0), [pickedSeats]);

  // ---- picking ----------------------------------------------------------
  const toggle = (seat) => {
    // A seat held by someone else or already sold is simply not selectable.
    if (seat.availability === "booked" || seat.availability === "held") {
      setNotice(`Seat ${seat.seatLabel} is already taken.`);
      return;
    }
    setLost([]);
    setNotice("");
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(seat.seatId)) {
        next.delete(seat.seatId);
      } else {
        if (next.size >= (map?.maxSeatsPerBooking || 10)) {
          setNotice(`You can book up to ${map.maxSeatsPerBooking} seats at a time.`);
          return prev;
        }
        next.add(seat.seatId);
      }
      return next;
    });
  };

  // Claim the selection. This is the moment the seats become exclusively ours.
  const claim = async () => {
    setBusy(true);
    setError("");
    setNotice("");
    setLost([]);
    try {
      const h = await ticketsApi.hold(eventId, [...picked]);
      setHold(h);
      setQuote({ seats: h.seats, totals: h.totals });
      setStep(1);
      await load(true);
    } catch (e) {
      // 409 means we lost the race — expected, not exceptional. Refresh so the
      // buyer sees the true state, and name the seats that went.
      if (e.status === 409) {
        setError(e.message);
        const taken = (e.message.match(/[A-Z]+\d+/g) || []);
        setLost(taken);
        setPicked((prev) => new Set([...prev].filter(
          (id) => !taken.includes(seatById.get(id)?.seatLabel))));
        await load(true);
      } else {
        setError(e.message);
      }
    } finally {
      setBusy(false);
    }
  };

  const giveUp = async () => {
    if (!hold) return;
    setBusy(true);
    try {
      await ticketsApi.release(hold.holdGroupId, eventId);
    } catch {
      // Releasing is best-effort: if it fails the hold simply times out.
    }
    setHold(null);
    setPicked(new Set());
    setQuote(null);
    setBooking(null);
    setIntent(null);
    setStep(0);
    setBusy(false);
    await load(true);
  };

  const applyCoupon = async () => {
    if (!hold) return;
    setBusy(true);
    setError("");
    try {
      setQuote(await ticketsApi.quote(hold.holdGroupId, coupon));
      setNotice(coupon ? `Coupon ${coupon.toUpperCase()} applied.` : "Coupon removed.");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- checkout ---------------------------------------------------------
  const startCheckout = async () => {
    setBusy(true);
    setError("");
    try {
      const b = await ticketsApi.createBooking(hold.holdGroupId, coupon);
      setBooking(b);
      setIntent(await ticketsApi.pay(b.id));
      setStep(2);
    } catch (e) {
      setError(e.message);
      // The hold lapsed underneath us — back to the picker.
      if (e.status === 409) await giveUp();
    } finally {
      setBusy(false);
    }
  };

  // settle drives the mock gateway. `success=false` exercises the retry path,
  // which is the same code a real declined card would take.
  const settle = async (success) => {
    setBusy(true);
    setError("");
    try {
      const res = await ticketsApi.confirm(booking.id, intent.providerRef, success);
      setBooking(res.booking || booking);

      switch (res.outcome) {
        case "confirmed":
        case "already_confirmed":
          setStep(3);
          break;
        case "retry":
          // Seats are still held; let them try again on the same booking.
          setError(res.message);
          setIntent(await ticketsApi.pay(booking.id));
          break;
        case "expired":
          setError(res.message);
          await giveUp();
          break;
        case "cancelled_refund_due":
          // The money was taken but the seats had gone. The server has already
          // cancelled and logged the refund; say so plainly.
          setError(res.message);
          setStep(4);
          break;
        default:
          setError(res.message || "Payment could not be completed.");
      }
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- render -----------------------------------------------------------
  if (loading) return <div className="spinner" />;

  if (!map) {
    return (
      <>
        {error && <div className="alert err">{error}</div>}
        <Button variant="o" onClick={() => navigate(`/events/${eventId}`)}>← Back to the event</Button>
      </>
    );
  }

  const urgent = remaining > 0 && remaining < 2 * 60 * 1000;
  const totals = quote?.totals;

  return (
    <>
      <div className="spread row" style={{ marginBottom: 14 }}>
        <div>
          <h1 style={{ fontSize: "1.5rem", color: "var(--ink)" }}>{map.eventName}</h1>
          <p className="muted" style={{ fontSize: ".85rem" }}>
            {map.hallName}, {map.hallCity} · {map.availableCount} of{" "}
            {map.availableCount + map.heldCount + map.bookedCount} seats available
          </p>
        </div>
        {hold && (
          <span className={`bkclock ${urgent ? "urgent" : ""}`}>
            ⏳ {mmss(remaining)} to complete payment
          </span>
        )}
      </div>

      <Stepper
        steps={["Choose seats", "Review", "Pay", "Done"]}
        current={Math.min(step, 3)}
      />

      {error && <div className="alert err">{error}</div>}
      {notice && !error && <div className="alert">{notice}</div>}

      {step === 0 && (
        <div className="bkwrap">
          <div>
            <div className="bkplan">
              <svg
                width={map.canvasWidth * zoomRef.current}
                height={map.canvasHeight * zoomRef.current}
                viewBox={`0 0 ${map.canvasWidth} ${map.canvasHeight}`}
              >
                {/* Stage, backstage and aisles — so the buyer knows which way
                    they will be facing and how far back a row is. */}
                {map.elements.map((el, i) => {
                  const light = el.kind === "aisle" || el.kind === "label";
                  return (
                    <g key={i}>
                      <rect
                        x={el.posX} y={el.posY} width={el.width} height={el.height}
                        rx={el.kind === "stage" ? 10 : 6}
                        fill={el.color} opacity={light ? 0.5 : 1}
                      />
                      {el.label && (
                        <text
                          x={el.posX + el.width / 2} y={el.posY + el.height / 2}
                          textAnchor="middle" dominantBaseline="central"
                          fontSize={Math.max(9, Math.min(15, el.height * 0.4))}
                          fontFamily="Poppins, sans-serif" fontWeight="700" letterSpacing="1.5"
                          fill={light ? "#4a4560" : "#fff"}
                        >
                          {el.label}
                        </text>
                      )}
                    </g>
                  );
                })}

                {map.seats.map((s) => {
                  const selected = picked.has(s.seatId);
                  const gone = s.availability === "booked" || s.availability === "held";
                  const justLost = lost.includes(s.seatLabel);
                  return (
                    <g key={s.seatId} className={`bkseat ${gone ? "gone" : ""}`}
                       onClick={() => toggle(s)}>
                      <title>
                        {`${s.section} · row ${s.rowLabel} · seat ${s.seatNumber}`}
                        {`\n${s.zoneName || "General"} · ${money(s.price)}`}
                        {gone ? "\nAlready taken" : ""}
                      </title>
                      <rect
                        x={s.posX - HALF} y={s.posY - HALF} width={SEAT} height={SEAT}
                        rx={s.kind === "wheelchair" ? 3 : 6}
                        fill={seatFill(s, selected)}
                        opacity={gone ? 0.75 : 1}
                        stroke={justLost ? "var(--red)" : selected ? "var(--gold)" : "rgba(0,0,0,.12)"}
                        strokeWidth={justLost || selected ? 3 : 1}
                      />
                      {s.kind === "wheelchair" && (
                        <text x={s.posX} y={s.posY} textAnchor="middle" dominantBaseline="central"
                              fontSize="11" fill="#fff" pointerEvents="none">♿</text>
                      )}
                      {s.availability === "booked" && (
                        <text x={s.posX} y={s.posY} textAnchor="middle" dominantBaseline="central"
                              fontSize="11" fontWeight="700" fill="#8a8598" pointerEvents="none">×</text>
                      )}
                    </g>
                  );
                })}
              </svg>
            </div>

            <div className="bklegend">
              {map.zones.map((z) => (
                <i key={z.code}><s style={{ background: z.color }} />{z.name} · {money(z.price)}</i>
              ))}
              <i><s style={{ background: "#0e1237" }} />Your selection</i>
              <i><s style={{ background: "#f3b0b3" }} />Being booked by someone else</i>
              <i><s style={{ background: "#d9d6e3" }} />Sold</i>
            </div>
            <p className="muted" style={{ fontSize: ".72rem", marginTop: 8 }}>
              Seats update automatically every few seconds. A seat someone else is
              paying for shows as unavailable until their {map.holdMinutes}-minute
              window closes.
            </p>
          </div>

          <Panel title={`Your seats (${picked.size})`}>
            {picked.size === 0 ? (
              <p className="muted" style={{ fontSize: ".85rem" }}>
                Tap seats on the plan to choose them. You can pick up to{" "}
                {map.maxSeatsPerBooking} at once.
              </p>
            ) : (
              <>
                {pickedSeats.map((s) => (
                  <div className="bkline" key={s.seatId}>
                    <span>
                      <b>{s.seatLabel}</b>
                      <span className="muted"> · {s.zoneName || "General"}</span>
                    </span>
                    <span>{money(s.price)}</span>
                  </div>
                ))}
                <div className="bktot">
                  <span className="muted">Subtotal</span>
                  <b>{money(pickedTotal)}</b>
                </div>
                <p className="muted" style={{ fontSize: ".72rem", marginTop: 10 }}>
                  Your seats are held for {map.holdMinutes} minutes once you continue,
                  giving you time to pay.
                </p>
                <Button block onClick={claim} disabled={busy} className="mt">
                  {busy ? "Holding your seats…" : `Hold ${picked.size} seat${picked.size === 1 ? "" : "s"}`}
                </Button>
              </>
            )}
          </Panel>
        </div>
      )}

      {step === 1 && totals && (
        <div className="bkwrap">
          <Panel title="Review your booking">
            {quote.seats.map((s) => (
              <div className="bkline" key={s.seatId}>
                <b>{s.seatLabel}</b>
                <span>{money(s.price)}</span>
              </div>
            ))}

            <div className="row mt" style={{ gap: 8 }}>
              <div className="f" style={{ flex: 1 }}>
                <input placeholder=" " value={coupon}
                       onChange={(e) => setCoupon(e.target.value.toUpperCase())} />
                <label>Coupon code</label>
              </div>
              <Button variant="o" onClick={applyCoupon} disabled={busy}>Apply</Button>
            </div>

            <div className="bkline mt">
              <span className="muted">Subtotal</span><span>{money(totals.subtotal)}</span>
            </div>
            {totals.discount > 0 && (
              <div className="bkline">
                <span className="muted">Discount</span>
                <span style={{ color: "var(--green)" }}>− {money(totals.discount)}</span>
              </div>
            )}
            <div className="bktot">
              <span className="muted">Total payable</span><b>{money(totals.total)}</b>
            </div>

            <Button block className="mt" onClick={startCheckout} disabled={busy}>
              {busy ? "Preparing payment…" : `Pay ${money(totals.total)}`}
            </Button>
            <Button variant="o" block className="mt" onClick={giveUp} disabled={busy}>
              Release seats and start over
            </Button>
          </Panel>

          <Panel title="Good to know">
            <p className="muted" style={{ fontSize: ".8rem", lineHeight: 1.6 }}>
              Your seats are held until the timer above runs out. If payment is not
              completed by then, the seats go back on sale for everyone else.
              <br /><br />
              Tickets can be cancelled from <b>My Bookings</b> up until the day the
              event starts. Refunds are processed <b>offline by our team</b> — they
              are not returned automatically to your payment method.
            </p>
          </Panel>
        </div>
      )}

      {step === 2 && intent && booking && (
        <Panel title="Payment">
          <p className="muted" style={{ fontSize: ".85rem" }}>
            Booking <b>{booking.reference}</b> · attempt {intent.attempt} of 3
          </p>
          <div className="bktot">
            <span className="muted">Amount</span><b>{money(intent.amount)}</b>
          </div>
          <p className="muted" style={{ fontSize: ".75rem", marginTop: 10 }}>
            This is a mock gateway for local use. Your seats stay held while the
            payment is processing, even if the timer runs out.
          </p>
          <Button block className="mt" onClick={() => settle(true)} disabled={busy}>
            {busy ? "Processing…" : "Pay now"}
          </Button>
          <Button variant="o" block className="mt" onClick={() => settle(false)} disabled={busy}>
            Simulate a failed payment
          </Button>
        </Panel>
      )}

      {step === 3 && booking && (
        <Panel title="You're booked 🎉">
          <p style={{ fontSize: ".9rem" }}>
            Booking reference <b>{booking.reference}</b>
          </p>
          <div className="bkseats">
            {booking.items?.map((it) => (
              <span className="bkchip" key={it.id}>{it.seatLabel}</span>
            ))}
          </div>
          <div className="bktot">
            <span className="muted">Paid</span><b>{money(booking.total)}</b>
          </div>
          <Button block className="mt" onClick={() => navigate("/my/bookings")}>
            View my bookings
          </Button>
        </Panel>
      )}

      {step === 4 && (
        <Panel title="Payment taken, seats unavailable">
          <p style={{ fontSize: ".88rem" }}>
            Your payment went through, but the seats were released before it
            completed and someone else took them. The booking has been cancelled
            and the full amount is logged for an <b>offline refund</b> by our team.
          </p>
          <Button block className="mt" onClick={() => navigate("/my/bookings")}>
            See the cancelled booking
          </Button>
          <Button variant="o" block className="mt" onClick={() => { setStep(0); load(); }}>
            Choose different seats
          </Button>
        </Panel>
      )}
    </>
  );
}
