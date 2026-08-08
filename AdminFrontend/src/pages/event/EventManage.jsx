import { useEffect, useState } from "react";
import { eventApi } from "../../api/endpoints";
import { downloadFile } from "../../api/client";
import { Button, Field, Modal, Alert, Badge, Empty } from "../../components/ui";

// EventManage is the per-event operations hub for the Event Admin, with five
// tabs: add ad-hoc (offline) participants, view/add crew, configure and send
// notifications, declare winners / issue certificates, and export the report.
const TABS = ["Add participant", "Crew", "Notifications", "Winners", "Media", "Export"];

export default function EventManage({ event, onClose }) {
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
      {tab === 0 && <AddParticipant event={event} />}
      {tab === 1 && <CrewTab event={event} />}
      {tab === 2 && <NotificationsTab event={event} />}
      {tab === 3 && <WinnersTab event={event} />}
      {tab === 4 && <MediaTab event={event} />}
      {tab === 5 && <ExportTab event={event} />}
    </Modal>
  );
}

// ---- Tab 1: ad-hoc participant with offline payment --------------------------
function AddParticipant({ event }) {
  const [cats, setCats] = useState([]);
  const empty = { name: "", dob: "", gender: "", aadhaar: "", phone: "", eventCategoryId: "" };
  const [form, setForm] = useState(empty);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    eventApi.eventCategories.list(event.id).then((c) => setCats(c || [])).catch((e) => setError(e.message));
  }, [event.id]);

  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));
  const submit = async () => {
    setError(""); setMsg(""); setBusy(true);
    try {
      const res = await eventApi.addOffline(event.id, form);
      setMsg(`Added ${res.personName} · entry #${res.entryCode} (offline paid).`);
      setForm(empty);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <p className="muted mb" style={{ fontSize: ".82rem" }}>
        Register a walk-in participant who paid offline (cash / transfer). The entry is marked paid.
      </p>
      <Alert>{error}</Alert>
      <Alert tone="ok">{msg}</Alert>
      {cats.length === 0 ? (
        <Empty>Add an age group to this event first (Edit → Age groups).</Empty>
      ) : (
        <>
          <div className="frow">
            <Field label="Full name" value={form.name} onChange={set("name")} />
            <Field label="Date of birth" type="date" value={form.dob} onChange={set("dob")} />
            <Field as="select" label="Gender" value={form.gender} onChange={set("gender")}
              options={["female", "male", "other"].map((v) => ({ value: v, label: v }))} />
            <Field label="Aadhaar (optional)" value={form.aadhaar} onChange={set("aadhaar")} inputMode="numeric" maxLength={12} />
            <Field label="Family mobile" value={form.phone} onChange={set("phone")} inputMode="numeric" maxLength={10} />
            <Field as="select" label="Age group" value={form.eventCategoryId} onChange={set("eventCategoryId")}
              options={cats.map((c) => ({ value: c.id, label: `${c.categoryName} · ${c.ageBandLabel} · ₹${c.fee}` }))} />
          </div>
          <Button onClick={submit} disabled={busy || !form.eventCategoryId}>Add participant (offline paid)</Button>
        </>
      )}
    </div>
  );
}

// ---- Tab 2: crew -------------------------------------------------------------
const CREW_ROLES = ["Stage", "Registration", "Green Room", "AV", "Security", "Hospitality"];
function CrewTab({ event }) {
  const [crew, setCrew] = useState([]);
  const [form, setForm] = useState({ name: "", role: "Stage", contact: "" });
  const [error, setError] = useState("");

  const load = () => eventApi.crew.list(event.id).then((c) => setCrew(c || [])).catch((e) => setError(e.message));
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const add = async () => {
    setError("");
    try {
      await eventApi.crew.add(event.id, form);
      setForm({ name: "", role: "Stage", contact: "" });
      await load();
    } catch (e) { setError(e.message); }
  };
  const remove = async (id) => {
    try { await eventApi.crew.remove(id); await load(); } catch (e) { setError(e.message); }
  };

  return (
    <div>
      <Alert>{error}</Alert>
      {crew.length === 0 ? <Empty>No crew assigned yet.</Empty> : (
        <table>
          <thead><tr><th>Name</th><th>Role</th><th>Contact</th><th></th></tr></thead>
          <tbody>
            {crew.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td><td><Badge tone="purple">{c.role}</Badge></td><td>{c.contact}</td>
                <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(c.id)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        <div className="frow">
          <Field label="Name" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
          <Field as="select" label="Role" value={form.role} onChange={(v) => setForm((f) => ({ ...f, role: v }))}
            options={CREW_ROLES.map((r) => ({ value: r, label: r }))} />
          <Field label="Contact" value={form.contact} onChange={(v) => setForm((f) => ({ ...f, contact: v }))} />
        </div>
        <Button onClick={add} disabled={!form.name}>+ Add crew member</Button>
      </div>
    </div>
  );
}

// ---- Tab 3: notifications ----------------------------------------------------
const TRIGGERS = [
  { key: "registration_confirmed", label: "Registration confirmed" },
  { key: "payment_received", label: "Payment received" },
  { key: "schedule_change", label: "Schedule / venue change" },
  { key: "round_reminder", label: "Round reminder (24h)" },
  { key: "results_published", label: "Results published" },
];
const CHANNELS = [["inApp", "In-app"], ["email", "Email"], ["sms", "SMS"], ["whatsapp", "WhatsApp"]];

function NotificationsTab({ event }) {
  const [config, setConfig] = useState({});
  const [sent, setSent] = useState([]);
  const [bc, setBc] = useState({ audience: "all", title: "", message: "" });
  const [error, setError] = useState("");
  const [msg, setMsg] = useState("");

  const load = async () => {
    try {
      setConfig((await eventApi.notifications.getConfig(event.id)) || {});
      setSent((await eventApi.notifications.list(event.id)) || []);
    } catch (e) { setError(e.message); }
  };
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const toggle = (trigger, channel) =>
    setConfig((c) => ({ ...c, [trigger]: { ...(c[trigger] || {}), [channel]: !(c[trigger]?.[channel]) } }));

  const saveConfig = async () => {
    setError(""); setMsg("");
    try { await eventApi.notifications.setConfig(event.id, config); setMsg("Triggers saved."); }
    catch (e) { setError(e.message); }
  };
  const send = async () => {
    setError(""); setMsg("");
    try {
      await eventApi.notifications.send(event.id, bc);
      setMsg("Broadcast sent to participants.");
      setBc({ audience: "all", title: "", message: "" });
      await load();
    } catch (e) { setError(e.message); }
  };

  return (
    <div>
      <Alert>{error}</Alert>
      <Alert tone="ok">{msg}</Alert>

      <h4 style={{ color: "var(--navy)", marginBottom: 8 }}>Automatic triggers</h4>
      <table>
        <thead><tr><th>Trigger</th>{CHANNELS.map(([, l]) => <th key={l}>{l}</th>)}</tr></thead>
        <tbody>
          {TRIGGERS.map((t) => (
            <tr key={t.key}>
              <td>{t.label}</td>
              {CHANNELS.map(([ch]) => (
                <td key={ch}><input type="checkbox" checked={!!config[t.key]?.[ch]} onChange={() => toggle(t.key, ch)} /></td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <div className="mt"><Button variant="o" size="sm" onClick={saveConfig}>Save triggers</Button></div>

      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 14 }}>
        <h4 style={{ color: "var(--navy)", marginBottom: 8 }}>Send a broadcast to participants</h4>
        <div className="frow">
          <Field as="select" label="Audience" value={bc.audience} onChange={(v) => setBc((b) => ({ ...b, audience: v }))}
            options={[{ value: "all", label: "All participants" }, { value: "paid", label: "Paid only" }, { value: "pending", label: "Payment pending" }]} />
          <Field label="Title" value={bc.title} onChange={(v) => setBc((b) => ({ ...b, title: v }))} />
        </div>
        <Field as="textarea" label="Message" value={bc.message} onChange={(v) => setBc((b) => ({ ...b, message: v }))} />
        <Button onClick={send} disabled={!bc.title || !bc.message}>Send broadcast</Button>

        {sent.length > 0 && (
          <div className="mt">
            <p className="muted" style={{ fontSize: ".75rem", marginBottom: 6 }}>Recently sent</p>
            {sent.map((n) => (
              <div key={n.id} style={{ padding: "6px 0", borderBottom: "1px solid var(--line)" }}>
                <b style={{ fontSize: ".85rem" }}>{n.title}</b> <Badge tone="purple">{n.audience}</Badge>
                <div className="muted" style={{ fontSize: ".76rem" }}>{n.message} · {n.createdAt}</div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// ---- Tab 4: winners / certificates ------------------------------------------
const POSITIONS = [
  { value: "gold", label: "1st · Gold" },
  { value: "silver", label: "2nd · Silver" },
  { value: "bronze", label: "3rd · Bronze" },
  { value: "participation", label: "Participation" },
];
const posTone = { gold: "review", silver: "purple", bronze: "draft", participation: "draft" };

function WinnersTab({ event }) {
  const [participants, setParticipants] = useState([]);
  const [certs, setCerts] = useState([]);
  const [form, setForm] = useState({ personId: "", position: "gold", imageUrl: "" });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const all = (await eventApi.participants.list()) || [];
      setParticipants(all.filter((p) => p.eventName === event.name));
      setCerts((await eventApi.certificates.list(event.id)) || []);
    } catch (e) { setError(e.message); }
  };
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const onFile = (e) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => setForm((f) => ({ ...f, imageUrl: reader.result }));
    reader.readAsDataURL(file);
  };

  const issue = async () => {
    setError(""); setBusy(true);
    try {
      await eventApi.certificates.issue(event.id, form);
      setForm({ personId: "", position: "gold", imageUrl: "" });
      await load();
    } catch (e) { setError(e.message); } finally { setBusy(false); }
  };
  const remove = async (id) => {
    try { await eventApi.certificates.remove(id); await load(); } catch (e) { setError(e.message); }
  };

  return (
    <div>
      <Alert>{error}</Alert>
      <p className="muted mb" style={{ fontSize: ".82rem" }}>
        Declare a position and (optionally) upload a certificate image. It appears on the participant’s Certificates page.
      </p>
      <div className="frow">
        <Field as="select" label="Participant" value={form.personId} onChange={(v) => setForm((f) => ({ ...f, personId: v }))}
          options={participants.map((p) => ({ value: p.personId, label: `${p.name} · ${p.categoryName}` }))} />
        <Field as="select" label="Position" value={form.position} onChange={(v) => setForm((f) => ({ ...f, position: v }))}
          options={POSITIONS} />
      </div>
      <label className="muted" style={{ fontSize: ".78rem" }}>Certificate image (optional)</label>
      <input type="file" accept="image/*" onChange={onFile} style={{ display: "block", margin: "6px 0 10px" }} />
      {form.imageUrl && <img src={form.imageUrl} alt="preview" style={{ maxHeight: 80, borderRadius: 8, marginBottom: 10 }} />}
      <Button onClick={issue} disabled={busy || !form.personId}>Declare result</Button>

      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
        {certs.length === 0 ? <Empty>No results declared yet.</Empty> : (
          <table>
            <thead><tr><th>Participant</th><th>Category</th><th>Position</th><th>Cert</th><th></th></tr></thead>
            <tbody>
              {certs.map((c) => (
                <tr key={c.id}>
                  <td>{c.personName}</td><td>{c.categoryName}</td>
                  <td><Badge tone={posTone[c.position] || "purple"}>{c.position}</Badge></td>
                  <td>{c.fileUrl ? "🖼 image" : c.certCode}</td>
                  <td style={{ textAlign: "right" }}><Button variant="danger" size="sm" onClick={() => remove(c.id)}>Remove</Button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

// ---- Tab 5: media (photos / videos) -----------------------------------------
function MediaTab({ event }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = () => eventApi.media.list(event.id).then((m) => setItems(m || [])).catch((e) => setError(e.message));
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [event.id]);

  const onFile = async (e) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const kind = file.type.startsWith("video") ? "video" : "photo";
    setError(""); setBusy(true);
    try {
      await eventApi.media.upload(event.id, file, kind);
      await load();
    } catch (err) { setError(err.message); } finally { setBusy(false); e.target.value = ""; }
  };
  const remove = async (id) => {
    try { await eventApi.media.remove(id); await load(); } catch (e) { setError(e.message); }
  };

  return (
    <div>
      <p className="muted mb" style={{ fontSize: ".82rem" }}>
        Upload photos and videos for this event. They appear on every participant’s My Events page.
      </p>
      <Alert>{error}</Alert>
      <input type="file" accept="image/*,video/*" onChange={onFile} disabled={busy} style={{ marginBottom: 12 }} />
      {items.length === 0 ? <Empty>No media uploaded yet.</Empty> : (
        <div className="row" style={{ gap: 10, flexWrap: "wrap" }}>
          {items.map((m) => (
            <div key={m.id} style={{ width: 130 }}>
              {m.kind === "video" ? (
                <video src={m.url} controls style={{ width: 130, height: 90, borderRadius: 8, objectFit: "cover", background: "#000" }} />
              ) : (
                <img src={m.url} alt="" style={{ width: 130, height: 90, borderRadius: 8, objectFit: "cover" }} />
              )}
              <Button variant="danger" size="sm" block onClick={() => remove(m.id)} className="mt">Remove</Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ---- Tab 6: export -----------------------------------------------------------
function ExportTab({ event }) {
  const [error, setError] = useState("");
  const dl = async (format, ext) => {
    setError("");
    try {
      await downloadFile(`/admin/event/events/${event.id}/report?format=${format}`, `stagex-${event.name}-report.${ext}`);
    } catch (e) { setError(e.message); }
  };
  return (
    <div>
      <Alert>{error}</Alert>
      <p className="muted mb" style={{ fontSize: ".85rem" }}>
        Download the full event report — participant list (name, contact, Aadhaar) plus the crew list — for offline use.
      </p>
      <div className="row">
        <Button variant="g" onClick={() => dl("csv", "csv")}>Export Excel (CSV)</Button>
        <Button variant="p" onClick={() => dl("pdf", "pdf")}>Export PDF</Button>
      </div>
    </div>
  );
}
