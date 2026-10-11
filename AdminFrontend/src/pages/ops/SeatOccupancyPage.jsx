import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Alert, Badge, Button, Empty, Panel, Spinner } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

// =============================================================================
// Seat occupancy & reservation report — Operational Admin
//
// Answers, seat by seat: what is available, what has been PAID for, what is
// mid-payment, and what is BLOCKED — so Ops can arrange the hall against real
// occupancy rather than a total. Then generates the seat-reservation report,
// including the event's profit and loss with ticket income folded in.
//
// The plan is drawn from the same coordinates the designer produced, coloured
// by occupancy rather than by price tier: on this screen the question is "can I
// sell this seat", not "what does it cost".
// =============================================================================

const SEAT = 20;
const HALF = SEAT / 2;

const money = (n) => "₹" + Number(n || 0).toLocaleString("en-IN", { maximumFractionDigits: 0 });
const num = (n) => Number(n || 0).toLocaleString("en-IN");
const pct = (n) => `${Number(n || 0).toFixed(1)}%`;
const fmtDateTime = (d) =>
  new Date(d).toLocaleString("en-IN", {
    day: "numeric", month: "short", hour: "numeric", minute: "2-digit",
  });

// One colour per occupancy state, used by both the plan and the legend.
const OCC = {
  available: { fill: "#2ecc8a", label: "Available", tone: "open" },
  booked:    { fill: "#6a1b9a", label: "Booked (paid)", tone: "purple" },
  held:      { fill: "#ffc107", label: "Payment in progress", tone: "review" },
  blocked:   { fill: "#d9d6e3", label: "Blocked", tone: "draft" },
  house:     { fill: "#0e1237", label: "House / press hold", tone: "draft" },
};

export default function SeatOccupancyPage() {
  const { eventId } = useParams();
  const navigate = useNavigate();

  const [rep, setRep] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [filter, setFilter] = useState("booked");
  const [zoom, setZoom] = useState(0.9);

  const load = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      setRep(await opsApi.seatOccupancy(eventId));
      setError("");
    } catch (e) {
      setError(e.message);
    } finally {
      if (!quiet) setLoading(false);
    }
  }, [eventId]);

  useEffect(() => { load(); }, [load]);

  // Held seats expire on their own, so a stale screen would misreport what is
  // actually sellable right now.
  useEffect(() => {
    const t = setInterval(() => load(true), 20000);
    return () => clearInterval(t);
  }, [load]);

  const download = async (format) => {
    setBusy(format);
    setError("");
    try {
      await opsApi.seatReport(eventId, format);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  };

  const seats = rep?.seats || [];
  const filtered = useMemo(
    () => (filter === "all" ? seats : seats.filter((s) => s.status === filter)),
    [seats, filter]
  );

  if (loading) return <Spinner />;

  if (!rep) {
    return (
      <>
        <div className="ptitle">
          <div><h1>Seat occupancy</h1></div>
          <Button variant="o" onClick={() => navigate("/ops/ticket-sales")}>← Ticket Sales</Button>
        </div>
        <Alert>{error || "Could not load this event."}</Alert>
      </>
    );
  }

  const s = rep.summary;
  const p = rep.pnl;
  const profit = p.netPL >= 0;

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>{rep.eventName} — seats</h1>
          <p>
            {rep.hallName}, {rep.city} ·{" "}
            {rep.mapStatus === "published"
              ? <Badge tone="open">arrangement published</Badge>
              : <Badge tone="draft">arrangement draft</Badge>}
            {" "}· generated {fmtDateTime(rep.generatedAt)}
          </p>
        </div>
        <div className="smbar">
          <Button variant="o" size="sm" onClick={() => navigate("/ops/ticket-sales")}>← Ticket Sales</Button>
          <Button variant="o" onClick={() => load()}>Refresh</Button>
          <Button variant="o" disabled={busy === "csv"} onClick={() => download("csv")}>
            {busy === "csv" ? "Building…" : "⬇ Report (CSV)"}
          </Button>
          <Button variant="g" disabled={busy === "pdf"} onClick={() => download("pdf")}>
            {busy === "pdf" ? "Building…" : "⬇ Report (PDF)"}
          </Button>
        </div>
      </div>

      <Alert>{error}</Alert>

      {/* The headline answer: how many seats are in each state. */}
      <div className="kpis">
        <div className="kpi">
          <div className="n" style={{ color: OCC.available.fill }}>{num(s.available)}</div>
          <div className="l">AVAILABLE</div>
        </div>
        <div className="kpi">
          <div className="n" style={{ color: OCC.booked.fill }}>{num(s.booked)}</div>
          <div className="l">BOOKED &amp; PAID</div>
        </div>
        <div className="kpi">
          <div className="n" style={{ color: "var(--gold-600)" }}>{num(s.held)}</div>
          <div className="l">PAYMENT IN PROGRESS</div>
        </div>
        <div className="kpi">
          <div className="n">{num(s.blocked + s.house)}</div>
          <div className="l">BLOCKED / HELD BACK</div>
        </div>
      </div>

      <div className="smwrap">
        <div className="smcol">
          <div className="smstage">
            <svg
              width={rep.canvasWidth * zoom}
              height={rep.canvasHeight * zoom}
              viewBox={`0 0 ${rep.canvasWidth} ${rep.canvasHeight}`}
            >
              {rep.elements.map((el, i) => {
                const light = el.kind === "aisle" || el.kind === "label";
                return (
                  <g key={i}>
                    <rect x={el.posX} y={el.posY} width={el.width} height={el.height}
                          rx={el.kind === "stage" ? 10 : 6}
                          fill={el.color} opacity={light ? 0.5 : 1} />
                    {el.label && (
                      <text x={el.posX + el.width / 2} y={el.posY + el.height / 2}
                            textAnchor="middle" dominantBaseline="central"
                            fontSize={Math.max(9, Math.min(15, el.height * 0.4))}
                            fontFamily="Poppins, sans-serif" fontWeight="700" letterSpacing="1.5"
                            fill={light ? "#4a4560" : "#fff"}>
                        {el.label}
                      </text>
                    )}
                  </g>
                );
              })}

              {seats.map((seat) => {
                const o = OCC[seat.status] || OCC.available;
                const highlighted = filter !== "all" && seat.status === filter;
                return (
                  <g key={seat.seatId}>
                    <title>
                      {`${seat.section} · row ${seat.rowLabel} (${seat.rowIndex} from stage) · seat ${seat.seatNumber}`}
                      {`\n${seat.zoneName || "No tier"} · list ${money(seat.listPrice)}`}
                      {`\n${o.label}`}
                      {seat.status === "booked"
                        ? `\nPaid ${money(seat.pricePaid)} · ${seat.bookingRef}\n${seat.buyerName || ""} ${seat.buyerPhone || ""}`
                        : ""}
                      {seat.status === "held"
                        ? `\nHold expires ${fmtDateTime(seat.holdExpiresAt)}${seat.paymentInFlight ? " · paying now" : ""}`
                        : ""}
                    </title>
                    <rect
                      x={seat.posX - HALF} y={seat.posY - HALF}
                      width={SEAT} height={SEAT}
                      rx={seat.kind === "wheelchair" ? 3 : 6}
                      fill={o.fill}
                      opacity={filter === "all" || highlighted ? 1 : 0.28}
                      stroke={highlighted ? "var(--navy)" : "rgba(0,0,0,.12)"}
                      strokeWidth={highlighted ? 2 : 1}
                    />
                    {seat.status === "blocked" && (
                      <text x={seat.posX} y={seat.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="11" fontWeight="700" fill="#8a8598" pointerEvents="none">×</text>
                    )}
                    {seat.status === "house" && (
                      <text x={seat.posX} y={seat.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="10" fontWeight="700" fill="#fff" pointerEvents="none">H</text>
                    )}
                    {/* A seat actively being paid for is the one Ops must not
                        reassign, so it gets a visible marker. */}
                    {seat.status === "held" && seat.paymentInFlight && (
                      <text x={seat.posX} y={seat.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="10" fontWeight="700" fill="#7a5a00" pointerEvents="none">₹</text>
                    )}
                  </g>
                );
              })}
            </svg>
          </div>

          <div className="smcard">
            <div className="smbody">
              <div className="smlegend">
                {Object.entries(OCC).map(([key, v]) => (
                  <i key={key}><s style={{ background: v.fill }} />{v.label}</i>
                ))}
                <i>₹ = payment in flight</i>
              </div>
              <div className="smbar">
                <span className="smhint">Highlight:</span>
                {["all", "available", "booked", "held", "blocked", "house"].map((k) => (
                  <Button key={k} variant={filter === k ? "p" : "o"} size="sm"
                          onClick={() => setFilter(k)}>
                    {k === "all" ? "All" : (OCC[k]?.label || k)}
                  </Button>
                ))}
                <span className="grow" />
                <select value={String(zoom)} onChange={(e) => setZoom(Number(e.target.value))}
                        style={{ border: "1.5px solid var(--line)", borderRadius: 9, padding: "6px 9px", fontSize: ".78rem" }}>
                  {[0.6, 0.75, 0.9, 1, 1.3].map((z) => (
                    <option key={z} value={z}>{Math.round(z * 100)}%</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        </div>

        {/* Money and P&L rail. */}
        <div className="smcol">
          <div className="smcard">
            <h4>Seat value</h4>
            <div className="smbody">
              <div className="smstats">
                <div className="smstat"><b>{money(s.realised)}</b><span>REALISED (PAID)</span></div>
                <div className="smstat"><b>{money(s.listValue)}</b><span>FULL-HOUSE VALUE</span></div>
                <div className="smstat"><b>{money(s.heldValue)}</b><span>IN PROGRESS</span></div>
                <div className="smstat"><b>{money(s.unsoldValue)}</b><span>STILL UNSOLD</span></div>
              </div>
              <div className="smstat"><b>{pct(s.occupancyPct)}</b><span>OCCUPANCY (BOOKED / SELLABLE)</span></div>
              <div className="smstat"><b>{pct(s.fillRatePct)}</b><span>FILL RATE (REALISED / LIST)</span></div>
              <p className="smhint">
                {num(s.sellable)} of {num(s.mappedSeats)} mapped seats are sellable
                ({num(s.blocked)} blocked, {num(s.house)} held back).
                {rep.hallCapacity > 0 && s.mappedSeats > rep.hallCapacity && (
                  <> ⚠ More mapped seats than the hall's registered capacity of {num(rep.hallCapacity)}.</>
                )}
              </p>
            </div>
          </div>

          <div className="smcard">
            <h4>Ticket sales</h4>
            <div className="smbody">
              <div className="smstats">
                <div className="smstat"><b>{num(rep.tickets.ticketsSold)}</b><span>TICKETS SOLD</span></div>
                <div className="smstat"><b>{num(rep.tickets.ticketsCancelled)}</b><span>CANCELLED</span></div>
              </div>
              <div className="smstat"><b>{money(rep.tickets.gross)}</b><span>GROSS COLLECTED</span></div>
              <div className="smstat"><b>− {money(rep.tickets.refunds)}</b><span>REFUNDS (OFFLINE)</span></div>
              <div className="smstat"><b>{money(rep.tickets.net)}</b><span>NET TICKET INCOME</span></div>
              {rep.tickets.refundsPending > 0 && (
                <div className="smstat warn">
                  <b>{money(rep.tickets.refundsPending)}</b><span>REFUND STILL OWED</span>
                </div>
              )}
            </div>
          </div>

          <div className="smcard">
            <h4>Profit &amp; loss (whole event)</h4>
            <div className="smbody">
              {[
                ["Registration revenue", p.revenue],
                ["Ticket income (net)", p.ticketIncome],
                ["Sponsor income", p.sponsorIncome],
                ["Vendor income", p.vendorIncome],
              ].map(([label, v]) => (
                <div className="smzone" key={label}>
                  <span className="nm">{label}</span>
                  <span>{money(v)}</span>
                </div>
              ))}
              <div className="smzone" style={{ borderTop: "1.5px solid var(--line)" }}>
                <span className="nm"><b>Total income</b></span>
                <span><b>{money(p.totalIncome)}</b></span>
              </div>
              {[
                ["Hall cost", p.hallCost],
                ["Crew cost", p.crewCost],
                ["Other expenses", p.expenses],
              ].map(([label, v]) => (
                <div className="smzone" key={label}>
                  <span className="nm">{label}</span>
                  <span>− {money(v)}</span>
                </div>
              ))}
              <div className="smzone" style={{ borderTop: "1.5px solid var(--line)" }}>
                <span className="nm"><b>Total expenses</b></span>
                <span><b>− {money(p.totalExpenses)}</b></span>
              </div>
              <div className="smstat" style={{ background: profit ? "#e6f9f0" : "#fff5f5" }}>
                <b style={{ color: profit ? "#1d9e69" : "var(--red)" }}>
                  {profit ? "" : "− "}{money(Math.abs(p.netPL))}
                </b>
                <span>NET {profit ? "PROFIT" : "LOSS"} · MARGIN {pct(p.margin)}</span>
              </div>
              {p.refundsPending > 0 && (
                <p className="smhint">
                  Excludes {money(p.refundsPending)} of refunds still owed — a
                  liability until finance settles it offline.
                </p>
              )}
            </div>
          </div>
        </div>
      </div>

      <Panel title="By price tier">
        <table>
          <thead>
            <tr>
              <th>Tier</th><th>Seats</th><th>Booked</th><th>In progress</th>
              <th>Available</th><th>Blocked</th><th>List value</th><th>Realised</th>
            </tr>
          </thead>
          <tbody>
            {rep.byZone.map((z) => (
              <tr key={z.code || "untiered"}>
                <td>
                  <span style={{
                    display: "inline-block", width: 11, height: 11, borderRadius: 3,
                    background: z.color, marginRight: 7,
                  }} />
                  <b>{z.name}</b>
                </td>
                <td>{num(z.seats)}</td>
                <td>{num(z.booked)}</td>
                <td>{z.held > 0 ? <Badge tone="review">{z.held}</Badge> : "—"}</td>
                <td>{num(z.available)}</td>
                <td>{z.blocked > 0 ? num(z.blocked) : "—"}</td>
                <td>{money(z.listValue)}</td>
                <td><b>{money(z.realised)}</b></td>
              </tr>
            ))}
          </tbody>
        </table>
      </Panel>

      {rep.cancellations.length > 0 && (
        <Panel title="Why tickets were cancelled">
          <table>
            <thead><tr><th>Reason</th><th>Bookings</th><th>Refund amount</th></tr></thead>
            <tbody>
              {rep.cancellations.map((c) => (
                <tr key={c.reason}>
                  <td>{c.reason.replace(/_/g, " ")}</td>
                  <td>{num(c.count)}</td>
                  <td>{money(c.amount)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      )}

      <Panel
        title={`Seat detail — ${filter === "all" ? "all seats" : OCC[filter]?.label || filter} (${filtered.length})`}
        actions={
          <span className="muted" style={{ fontSize: ".76rem" }}>
            Use the highlight buttons above to change this list
          </span>
        }
      >
        {filtered.length === 0 ? (
          <Empty>No seats in this state.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Seat</th><th>Section</th><th>Rows from stage</th><th>Tier</th>
                <th>List price</th><th>State</th><th>Paid</th><th>Booked by</th><th>Reference</th>
              </tr>
            </thead>
            <tbody>
              {/* Capped: a 500-seat hall would otherwise render a wall of rows.
                  The CSV export is the complete record. */}
              {filtered.slice(0, 200).map((seat) => (
                <tr key={seat.seatId}>
                  <td><b>{seat.seatLabel}</b></td>
                  <td>{seat.section}</td>
                  <td>{seat.rowIndex}</td>
                  <td>{seat.zoneName || <span className="muted">no tier</span>}</td>
                  <td>{money(seat.listPrice)}</td>
                  <td>
                    <Badge tone={OCC[seat.status]?.tone || "purple"}>
                      {OCC[seat.status]?.label || seat.status}
                    </Badge>
                    {seat.paymentInFlight && <span className="muted"> paying now</span>}
                  </td>
                  <td>{seat.status === "booked" ? <b>{money(seat.pricePaid)}</b> : "—"}</td>
                  <td>
                    {seat.buyerName || "—"}
                    {seat.buyerPhone && (
                      <><br /><span className="muted" style={{ fontSize: ".72rem" }}>{seat.buyerPhone}</span></>
                    )}
                  </td>
                  <td>
                    <span className="muted" style={{ fontFamily: "Poppins", fontSize: ".74rem" }}>
                      {seat.bookingRef || "—"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {filtered.length > 200 && (
          <p className="muted" style={{ fontSize: ".76rem", padding: "10px 4px 0" }}>
            Showing the first 200 of {filtered.length}. Download the CSV report for every seat.
          </p>
        )}
      </Panel>
    </>
  );
}
