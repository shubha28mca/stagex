import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { eventsApi, peopleApi, registrationsApi, couponsApi, paymentsApi } from "../api/endpoints";
import { useAuth } from "../context/AuthContext";
import { Button, Panel, Field, Stepper, Badge, Spinner, Alert, Empty, initials } from "../components";

const STEPS = ["Participants", "Payment", "Confirmed"];

// EventDetailPage hosts the full 3-step registration flow (ClientDesignWeb §5):
// choose participants (with per-person eligibility) → payment (coupon + retry)
// → confirmation with entry IDs.
export default function EventDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { isAuthenticated } = useAuth();

  const [event, setEvent] = useState(null);
  const [people, setPeople] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [step, setStep] = useState(0);

  // Step 0 state: personId -> eventCategoryId selection.
  const [selection, setSelection] = useState({});
  const [couponCode, setCouponCode] = useState("");
  const [couponPreview, setCouponPreview] = useState(null);

  // Steps 1–2 state.
  const [registration, setRegistration] = useState(null);
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const ev = await eventsApi.get(id);
        setEvent(ev);
        if (isAuthenticated) setPeople((await peopleApi.list()) || []);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, [id, isAuthenticated]);

  // Categories a given person is age-eligible for.
  const eligibleCategories = (person) =>
    (event?.categories || []).filter(
      (c) => person.ageYears >= c.minAge && person.ageYears <= c.maxAge
    );

  const selectedEntries = useMemo(
    () =>
      Object.entries(selection)
        .filter(([, ecId]) => ecId)
        .map(([personId, eventCategoryId]) => ({ personId, eventCategoryId })),
    [selection]
  );

  const subtotal = useMemo(() => {
    return selectedEntries.reduce((sum, { eventCategoryId }) => {
      const c = event?.categories?.find((x) => x.id === eventCategoryId);
      return sum + (c?.fee || 0);
    }, 0);
  }, [selectedEntries, event]);

  const togglePerson = (person) => {
    setSelection((prev) => {
      const next = { ...prev };
      if (next[person.id]) {
        delete next[person.id];
      } else {
        const first = eligibleCategories(person)[0];
        next[person.id] = first ? first.id : "";
      }
      return next;
    });
  };

  const applyCoupon = async () => {
    setError("");
    try {
      const res = await couponsApi.validate(couponCode, subtotal, id);
      setCouponPreview(res);
      if (!res.valid) setError(`Coupon: ${res.reason}`);
    } catch (e) {
      setError(e.message);
    }
  };

  const proceedToPayment = async () => {
    setError("");
    setBusy(true);
    try {
      const reg = await registrationsApi.create({
        eventId: id,
        couponCode: couponPreview?.valid ? couponCode : "",
        entries: selectedEntries,
      });
      setRegistration(reg);
      setStep(1);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // pay(success) creates an order then confirms it. Passing success=false walks
  // the 3-attempt retry counter (ClientDesignWeb §5.2).
  const pay = async (success) => {
    setError("");
    setBusy(true);
    try {
      const order = await paymentsApi.createOrder(registration.id);
      setAttempt(order.attempt);
      const res = await paymentsApi.confirm(registration.id, success);
      if (res.status === "paid") {
        setStep(2);
      } else if (res.status === "locked") {
        setError("Payment locked after 3 attempts — try a different method or contact support.");
      } else {
        setError(`Payment failed (attempt ${res.attempt}). ${res.attemptsLeft} attempt(s) left.`);
      }
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Spinner />;
  if (!event) return <Empty>Event not found.</Empty>;

  return (
    <>
      <div
        className={`cov grad-${event.coverGradient || "purple"}`}
        style={{ borderRadius: "var(--r-lg)", padding: 22, color: "#fff", marginBottom: 16, minHeight: 90 }}
      >
        <Badge tone="gold">{event.eventType || event.mode}</Badge>
        <h1 style={{ marginTop: 8 }}>{event.name}</h1>
        <p style={{ opacity: 0.85 }}>
          📍 {event.city} · {event.rounds} rounds · {event.mode} ·{" "}
          {event.fee > 0 ? `from ₹${event.fee}` : "Free"}
        </p>
      </div>

      <EventAbout event={event} />

      {!isAuthenticated ? (
        <Panel title="Login to register">
          <p className="muted mb">You need a family account to register participants.</p>
          <Button variant="g" onClick={() => navigate("/auth")}>Login / Register</Button>
        </Panel>
      ) : (
        <>
          <Stepper steps={STEPS} current={step} />
          <Alert>{error}</Alert>

          {step === 0 && (
            <Panel
              title="Choose participants"
              actions={<span className="muted">Subtotal: <b>₹{subtotal}</b></span>}
            >
              {people.filter((p) => !p.deleted).length === 0 ? (
                <Empty>
                  {people.length === 0 ? "No people yet. " : "No available people. "}
                  <a style={{ color: "var(--purple)", cursor: "pointer" }} onClick={() => navigate("/my/people")}>
                    {people.length === 0 ? "Add someone in My People" : "Manage My People"}
                  </a>{" "}
                  first.
                </Empty>
              ) : (
                people
                  .filter((p) => !p.deleted)
                  .map((p) => {
                    const eligible = eligibleCategories(p);
                    const on = !!selection[p.id];
                    return (
                      <div key={p.id}>
                        <div className={`pcard ${on ? "on" : ""}`}>
                          <span className="ck" onClick={() => eligible.length && togglePerson(p)}>
                            {on ? "✓" : ""}
                          </span>
                          <span className="pavatar">{initials(p.name)}</span>
                          <div style={{ flex: 1 }}>
                            <b>{p.name}</b>
                            <div className="muted" style={{ fontSize: ".78rem" }}>
                              Age {p.ageYears} · {p.relationship}
                            </div>
                          </div>
                          {eligible.length === 0 ? (
                            <span className="badge b-err">Not eligible</span>
                          ) : on ? (
                            <select
                              value={selection[p.id]}
                              onChange={(e) => setSelection((s) => ({ ...s, [p.id]: e.target.value }))}
                              style={{ borderRadius: 10, padding: "8px 10px", border: "1.5px solid var(--line)" }}
                            >
                              {eligible.map((c) => (
                                <option key={c.id} value={c.id}>
                                  {c.categoryName} (₹{c.fee})
                                </option>
                              ))}
                            </select>
                          ) : null}
                        </div>

                        {eligible.length === 0 && (event.categories || []).length > 0 && (
                          <div className="alert err" style={{ marginTop: -4 }}>
                            <b>Why {p.name} can’t be entered</b>
                            <div style={{ marginTop: 4 }}>
                              {p.name} is <b>{p.ageYears}</b> years old, but every category in this
                              event is limited to a specific age band:
                            </div>
                            <ul style={{ margin: "6px 0 6px 18px" }}>
                              {event.categories.map((c) => {
                                const gap =
                                  p.ageYears < c.minAge
                                    ? `needs to be ${c.minAge - p.ageYears} year(s) older`
                                    : `over the limit by ${p.ageYears - c.maxAge} year(s)`;
                                return (
                                  <li key={c.id}>
                                    {c.categoryName}: ages {c.minAge}–{c.maxAge} — {gap}
                                  </li>
                                );
                              })}
                            </ul>
                            <div>
                              What to do: if the date of birth is wrong,{" "}
                              <a
                                style={{ color: "var(--purple)", cursor: "pointer", fontWeight: 600 }}
                                onClick={() => navigate("/my/people")}
                              >
                                edit it in My People
                              </a>
                              . Otherwise this event has no category open for this age — try another
                              event on Discover.
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })
              )}

              <div className="grid2 mt">
                <Field label="Coupon code" value={couponCode} onChange={setCouponCode} />
                <div>
                  <Button variant="o" onClick={applyCoupon} disabled={!couponCode || subtotal === 0}>
                    Apply coupon
                  </Button>
                  {couponPreview?.valid && (
                    <div className="fmsg ok">
                      −₹{couponPreview.discount} applied · new total ₹{couponPreview.total}
                    </div>
                  )}
                </div>
              </div>

              <div className="mt">
                <Button
                  variant="g"
                  block
                  disabled={selectedEntries.length === 0 || busy}
                  onClick={proceedToPayment}
                >
                  Continue to payment · ₹{couponPreview?.valid ? couponPreview.total : subtotal}
                </Button>
              </div>
            </Panel>
          )}

          {step === 1 && registration && (
            <Panel title="Payment">
              {registration.entries.map((en) => (
                <div className="row spread" key={en.id} style={{ padding: "6px 0", borderBottom: "1px solid var(--line)" }}>
                  <span>{en.personName} · {en.categoryName}</span>
                  <b>₹{en.fee}</b>
                </div>
              ))}
              <div className="row spread mt">
                <span className="muted">Subtotal</span><span>₹{registration.subtotal}</span>
              </div>
              {registration.discount > 0 && (
                <div className="row spread">
                  <span className="muted">Discount ({registration.couponCode})</span>
                  <span style={{ color: "var(--green)" }}>−₹{registration.discount}</span>
                </div>
              )}
              <div className="row spread" style={{ fontSize: "1.1rem", marginTop: 8 }}>
                <b>Total</b><b>₹{registration.total}</b>
              </div>

              {attempt > 0 && <p className="muted mt">Attempt {attempt} of 3</p>}

              <div className="row mt">
                <Button variant="g" onClick={() => pay(true)} disabled={busy}>
                  Pay ₹{registration.total} now
                </Button>
                <Button variant="o" onClick={() => pay(false)} disabled={busy}>
                  Simulate failed attempt
                </Button>
              </div>
            </Panel>
          )}

          {step === 2 && registration && (
            <Panel title="🎉 Registration confirmed">
              <p className="mb">Paid ₹{registration.total}. Your entry IDs:</p>
              {registration.entries.map((en) => (
                <div className="row spread" key={en.id} style={{ padding: "8px 0", borderBottom: "1px solid var(--line)" }}>
                  <span>{en.personName} · {en.categoryName}</span>
                  <Badge tone="purple">#{en.entryCode}</Badge>
                </div>
              ))}
              <div className="row mt">
                <Button variant="g" onClick={() => navigate("/my/events")}>Go to My Events</Button>
                <Button variant="o" onClick={() => navigate("/discover")}>Discover more</Button>
              </div>
            </Panel>
          )}
        </>
      )}
    </>
  );
}

// EventAbout renders the rich, public event detail (rounds, judging rubric,
// judges, categories & fees, capacity) so participants can decide before they
// register — the deeper view behind the Discover card's "View details" link.
function EventAbout({ event }) {
  const pct = event.slotsTotal ? Math.round((event.slotsFilled / event.slotsTotal) * 100) : 0;
  return (
    <Panel title="About this event">
      {event.tagline && <p className="mb">{event.tagline}</p>}

      <div className="grid2">
        <div>
          <h4 style={{ color: "var(--navy)", marginBottom: 6 }}>Rounds</h4>
          {event.roundsDetail?.length ? (
            <ol style={{ margin: "0 0 12px 18px" }}>
              {event.roundsDetail.map((r, i) => (
                <li key={i} style={{ marginBottom: 4 }}>
                  <b>{r.name}</b>
                  {r.description ? <span className="muted"> — {r.description}</span> : null}
                </li>
              ))}
            </ol>
          ) : (
            <p className="muted mb">{event.rounds} round{event.rounds > 1 ? "s" : ""}</p>
          )}

          <h4 style={{ color: "var(--navy)", margin: "8px 0 6px" }}>Judging rubric</h4>
          {event.rubric?.length ? (
            <ul style={{ margin: "0 0 12px 18px" }}>
              {event.rubric.map((c, i) => (
                <li key={i}>{c.criterion} <span className="muted">· {c.weight}%</span></li>
              ))}
            </ul>
          ) : (
            <p className="muted mb">Announced at the event.</p>
          )}

          {event.judges?.length > 0 && (
            <>
              <h4 style={{ color: "var(--navy)", margin: "8px 0 6px" }}>Judges</h4>
              <div className="row" style={{ gap: 8 }}>
                {event.judges.map((j, i) => <Badge key={i} tone="purple">{j}</Badge>)}
              </div>
            </>
          )}
        </div>

        <div>
          <h4 style={{ color: "var(--navy)", marginBottom: 6 }}>Categories & fees</h4>
          {event.categories?.length ? (
            event.categories.map((c) => (
              <div className="row spread" key={c.id} style={{ padding: "6px 0", borderBottom: "1px solid var(--line)" }}>
                <span>{c.categoryName} <span className="muted">· {c.ageBandLabel}</span></span>
                <b>₹{c.fee}</b>
              </div>
            ))
          ) : (
            <p className="muted">Categories announced soon.</p>
          )}

          <h4 style={{ color: "var(--navy)", margin: "12px 0 6px" }}>Capacity</h4>
          <div className="prog"><i style={{ width: `${pct}%` }} /></div>
          <p className="muted" style={{ fontSize: ".8rem", marginTop: 4 }}>
            {event.slotsFilled}/{event.slotsTotal} slots filled
          </p>
        </div>
      </div>
    </Panel>
  );
}
