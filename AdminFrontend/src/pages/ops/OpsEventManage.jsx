import { useEffect, useState } from "react";
import { opsApi } from "../../api/endpoints";
import { downloadFile } from "../../api/client";
import { Button, Field, Modal, Alert, Badge, Empty } from "../../components/ui";

// OpsEventManage is the Operations per-event hub: assign crew, book expenses,
// view P&L, export the report, and archive (download + purge) the event.
const TABS = ["Crew", "Vendors", "Sponsors", "Expenses", "P&L", "Export", "Archive"];

export default function OpsEventManage({ event, onClose, onArchived }) {
  const [tab, setTab] = useState(0);
  return (
    <Modal title={`Manage — ${event.name}`} onClose={onClose} footer={<Button variant="o" onClick={onClose}>Close</Button>}>
      <div className="row" style={{ gap: 6, marginBottom: 16 }}>
        {TABS.map((t, i) => (
          <span key={t} className={`chip ${i === tab ? "on" : ""}`} style={{ flex: 1, textAlign: "center", cursor: "pointer", fontSize: ".72rem" }} onClick={() => setTab(i)}>
            {t}
          </span>
        ))}
      </div>
      {tab === 0 && <CrewTab event={event} />}
      {tab === 1 && <VendorsTab event={event} />}
      {tab === 2 && <SponsorsTab event={event} />}
      {tab === 3 && <ExpensesTab event={event} />}
      {tab === 4 && <PnLTab event={event} />}
      {tab === 5 && <ExportTab event={event} />}
      {tab === 6 && <ArchiveTab event={event} onArchived={onArchived} />}
    </Modal>
  );
}

// ---- Crew: assign from the pool -------------------------------------------
function CrewTab({ event }) {
  const [assigned, setAssigned] = useState([]);
  const [pool, setPool] = useState([]);
  const [crewId, setCrewId] = useState("");
  const [error, setError] = useState("");

  const load = async () => {
    try {
      setAssigned((await opsApi.eventCrew.list(event.id)) || []);
      setPool((await opsApi.crew.list()) || []);
    } catch (e) { setError(e.message); }
  };
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const assign = async () => {
    setError("");
    try { await opsApi.eventCrew.assign(event.id, crewId); setCrewId(""); await load(); }
    catch (e) { setError(e.message); }
  };
  const remove = async (id) => {
    try { await opsApi.eventCrew.remove(id); await load(); } catch (e) { setError(e.message); }
  };
  const total = assigned.reduce((s, c) => s + (Number(c.cost) || 0), 0);

  return (
    <div>
      <Alert>{error}</Alert>
      {assigned.length === 0 ? <Empty>No crew assigned yet.</Empty> : (
        <table>
          <thead><tr><th>Name</th><th>Role</th><th>Cost ₹</th><th></th></tr></thead>
          <tbody>
            {assigned.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td><td><Badge tone="purple">{c.role}</Badge></td><td>{c.cost}</td>
                <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(c.id)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="muted mt" style={{ fontSize: ".8rem" }}>Total crew cost: <b>₹{total}</b></p>

      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        <div className="frow">
          <Field as="select" label="Assign from crew pool" value={crewId} onChange={setCrewId}
            options={pool.filter((p) => p.isActive).map((p) => ({ value: p.id, label: `${p.name} · ${p.role} · ₹${p.cost}` }))} />
          <div><Button onClick={assign} disabled={!crewId}>Assign to event</Button></div>
        </div>
      </div>
    </div>
  );
}

// ---- Vendors: assign from the pool with a cost (income) ---------------------
function VendorsTab({ event }) {
  const [assigned, setAssigned] = useState([]);
  const [pool, setPool] = useState([]);
  const [form, setForm] = useState({ vendorId: "", cost: 0 });
  const [error, setError] = useState("");

  const load = async () => {
    try {
      setAssigned((await opsApi.eventVendors.list(event.id)) || []);
      setPool((await opsApi.vendors.list()) || []);
    } catch (e) { setError(e.message); }
  };
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const assign = async () => {
    setError("");
    try { await opsApi.eventVendors.assign(event.id, form.vendorId, Number(form.cost)); setForm({ vendorId: "", cost: 0 }); await load(); }
    catch (e) { setError(e.message); }
  };
  const remove = async (id) => { try { await opsApi.eventVendors.remove(id); await load(); } catch (e) { setError(e.message); } };
  const total = assigned.reduce((s, v) => s + (Number(v.cost) || 0), 0);

  return (
    <div>
      <Alert>{error}</Alert>
      <p className="muted mb" style={{ fontSize: ".8rem" }}>Vendor cost is booked as income and added to the event profit.</p>
      {assigned.length === 0 ? <Empty>No vendors assigned yet.</Empty> : (
        <table>
          <thead><tr><th>Vendor</th><th>Service</th><th>Cost ₹</th><th></th></tr></thead>
          <tbody>
            {assigned.map((v) => (
              <tr key={v.id}>
                <td>{v.name}</td><td><Badge tone="purple">{v.serviceType}</Badge></td><td>{v.cost}</td>
                <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(v.id)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="muted mt" style={{ fontSize: ".8rem" }}>Total vendor income: <b>₹{total}</b></p>
      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        <div className="frow">
          <Field as="select" label="Vendor from pool" value={form.vendorId} onChange={(v) => setForm((f) => ({ ...f, vendorId: v }))}
            options={pool.filter((p) => p.isActive).map((p) => ({ value: p.id, label: `${p.name} · ${p.serviceType}` }))} />
          <Field label="Cost ₹ (income)" type="number" value={form.cost} onChange={(v) => setForm((f) => ({ ...f, cost: v }))} />
        </div>
        <Button onClick={assign} disabled={!form.vendorId}>Assign vendor</Button>
      </div>
    </div>
  );
}

// ---- Sponsors: assign from the pool with a cost (income) --------------------
function SponsorsTab({ event }) {
  const [assigned, setAssigned] = useState([]);
  const [pool, setPool] = useState([]);
  const [form, setForm] = useState({ sponsorId: "", cost: 0 });
  const [error, setError] = useState("");

  const load = async () => {
    try {
      setAssigned((await opsApi.eventSponsors.list(event.id)) || []);
      setPool((await opsApi.sponsors.list()) || []);
    } catch (e) { setError(e.message); }
  };
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const assign = async () => {
    setError("");
    try { await opsApi.eventSponsors.assign(event.id, form.sponsorId, Number(form.cost)); setForm({ sponsorId: "", cost: 0 }); await load(); }
    catch (e) { setError(e.message); }
  };
  const remove = async (id) => { try { await opsApi.eventSponsors.remove(id); await load(); } catch (e) { setError(e.message); } };
  const total = assigned.reduce((s, v) => s + (Number(v.cost) || 0), 0);

  return (
    <div>
      <Alert>{error}</Alert>
      <p className="muted mb" style={{ fontSize: ".8rem" }}>Sponsor contribution is added to the event profit.</p>
      {assigned.length === 0 ? <Empty>No sponsors assigned yet.</Empty> : (
        <table>
          <thead><tr><th>Sponsor</th><th>Tier</th><th>Cost ₹</th><th></th></tr></thead>
          <tbody>
            {assigned.map((v) => (
              <tr key={v.id}>
                <td>{v.organisation}</td><td><Badge tone="review">{v.tier}</Badge></td><td>{v.cost}</td>
                <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(v.id)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="muted mt" style={{ fontSize: ".8rem" }}>Total sponsor income: <b>₹{total}</b></p>
      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        <div className="frow">
          <Field as="select" label="Sponsor from pool" value={form.sponsorId} onChange={(v) => setForm((f) => ({ ...f, sponsorId: v }))}
            options={pool.map((p) => ({ value: p.id, label: `${p.organisation} · ${p.tier}` }))} />
          <Field label="Cost ₹ (income)" type="number" value={form.cost} onChange={(v) => setForm((f) => ({ ...f, cost: v }))} />
        </div>
        <Button onClick={assign} disabled={!form.sponsorId}>Assign sponsor</Button>
      </div>
    </div>
  );
}

// ---- Expenses --------------------------------------------------------------
function ExpensesTab({ event }) {
  const [rows, setRows] = useState([]);
  const [form, setForm] = useState({ amount: 0, comment: "" });
  const [error, setError] = useState("");

  const load = () => opsApi.expenses.list(event.id).then((r) => setRows(r || [])).catch((e) => setError(e.message));
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const add = async () => {
    setError("");
    try { await opsApi.expenses.add(event.id, { amount: Number(form.amount), comment: form.comment }); setForm({ amount: 0, comment: "" }); await load(); }
    catch (e) { setError(e.message); }
  };
  const remove = async (id) => { try { await opsApi.expenses.remove(id); await load(); } catch (e) { setError(e.message); } };
  const total = rows.reduce((s, r) => s + (Number(r.amount) || 0), 0);

  return (
    <div>
      <Alert>{error}</Alert>
      {rows.length === 0 ? <Empty>No additional expenses booked.</Empty> : (
        <table>
          <thead><tr><th>Amount ₹</th><th>Comment</th><th>Date</th><th></th></tr></thead>
          <tbody>
            {rows.map((e) => (
              <tr key={e.id}>
                <td>{e.amount}</td><td>{e.comment}</td><td>{e.createdAt}</td>
                <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(e.id)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="muted mt" style={{ fontSize: ".8rem" }}>Total additional expenses: <b>₹{total}</b></p>

      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        <div className="frow">
          <Field label="Amount ₹" type="number" value={form.amount} onChange={(v) => setForm((f) => ({ ...f, amount: v }))} />
          <Field label="Comment" value={form.comment} onChange={(v) => setForm((f) => ({ ...f, comment: v }))} />
        </div>
        <Button onClick={add} disabled={!form.amount}>+ Add expense</Button>
      </div>
    </div>
  );
}

// ---- P&L -------------------------------------------------------------------
function PnLTab({ event }) {
  const [p, setP] = useState(null);
  const [error, setError] = useState("");
  useEffect(() => { opsApi.pnl(event.id).then(setP).catch((e) => setError(e.message)); }, [event.id]);
  if (error) return <Alert>{error}</Alert>;
  if (!p) return <p className="muted">Loading…</p>;

  const rupee = (v) => `₹${Number(v).toLocaleString()}`;
  const rows = [
    ["Revenue (paid registrations)", rupee(p.revenue)],
    ["Sponsor income", rupee(p.sponsorIncome)],
    ["Vendor income", rupee(p.vendorIncome)],
    ["Total income", rupee(p.totalIncome)],
    ["Participants", p.participants],
    ["Crew cost", rupee(p.crewCost)],
    ["Additional expenses", rupee(p.expenses)],
    ["Hall cost", rupee(p.hallCost)],
    ["Total expenses", rupee(p.totalExpenses)],
  ];
  return (
    <div>
      {rows.map(([l, v]) => (
        <div className="row spread" key={l} style={{ padding: "7px 0", borderBottom: "1px solid var(--line)" }}>
          <span className="muted" style={{ fontSize: ".85rem" }}>{l}</span>
          <b style={{ fontSize: ".9rem" }}>{v}</b>
        </div>
      ))}
      <div className="row spread" style={{ padding: "12px 0 4px", fontSize: "1.1rem" }}>
        <b>Net {p.netPL >= 0 ? "Profit" : "Loss"}</b>
        <b style={{ color: p.netPL >= 0 ? "var(--green)" : "var(--red)" }}>
          {rupee(p.netPL)} ({p.margin.toFixed(1)}%)
        </b>
      </div>
    </div>
  );
}

// ---- Export ----------------------------------------------------------------
function ExportTab({ event }) {
  const [error, setError] = useState("");
  const dl = async (format, ext) => {
    setError("");
    try { await downloadFile(`/admin/ops/events/${event.id}/report?format=${format}`, `stagex-${event.name}-ops-report.${ext}`); }
    catch (e) { setError(e.message); }
  };
  return (
    <div>
      <Alert>{error}</Alert>
      <p className="muted mb" style={{ fontSize: ".85rem" }}>
        Full event report — P&L, participants (name, contact, Aadhaar), crew and expenses.
      </p>
      <div className="row">
        <Button variant="g" onClick={() => dl("csv", "csv")}>Export Excel (CSV)</Button>
        <Button variant="p" onClick={() => dl("pdf", "pdf")}>Export PDF</Button>
      </div>
    </div>
  );
}

// ---- Archive (download + purge) --------------------------------------------
function ArchiveTab({ event, onArchived }) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const archive = async () => {
    if (!window.confirm(`Archive "${event.name}"? This downloads an archive file and permanently removes ALL of the event's data.`)) return;
    setError(""); setBusy(true);
    try {
      await downloadFile(`/admin/ops/events/${event.id}/archive`, `stagex-archive-${event.name}.json`, "POST");
      onArchived?.();
    } catch (e) { setError(e.message); } finally { setBusy(false); }
  };
  return (
    <div>
      <Alert>{error}</Alert>
      <div className="alert err" style={{ background: "#fff0e6", color: "var(--orange)" }}>
        Archiving downloads a complete JSON archive of the event (participants, crew, expenses, P&L)
        and then <b>permanently deletes</b> all of its data from the platform.
      </div>
      <Button variant="danger" onClick={archive} disabled={busy}>Archive &amp; download</Button>
    </div>
  );
}
