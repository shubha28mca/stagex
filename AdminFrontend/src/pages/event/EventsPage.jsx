import { useEffect, useState } from "react";
import { eventApi } from "../../api/endpoints";
import { Button, Panel, Field, Modal, Badge, Spinner, Alert, Empty } from "../../components/ui";
import EventWizard from "./EventWizard";
import EventManage from "./EventManage";

const tone = { open: "open", live: "live", completed: "draft", draft: "draft" };

// EventsPage — the Event Admin's own events. Creation/editing runs through the
// multi-step EventWizard (Details → Rounds → Rubric → Fees & capacity → Review).
export default function EventsPage() {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [editing, setEditing] = useState(null); // null | {} (new) | row (edit)
  const [catEvent, setCatEvent] = useState(null); // event whose categories we manage
  const [manageEvent, setManageEvent] = useState(null); // event ops hub

  const load = async () => {
    setLoading(true);
    try {
      setEvents((await eventApi.events.list()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    load();
  }, []);

  const publish = async (e) => {
    try {
      await eventApi.events.publish(e.id);
      await load();
    } catch (err) {
      setError(err.message);
    }
  };
  const remove = async (e) => {
    if (!window.confirm(`Delete "${e.name}" and all its registrations?`)) return;
    try {
      await eventApi.events.remove(e.id);
      await load();
    } catch (err) {
      setError(err.message);
    }
  };

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>My Events</h1>
          <p>Create, publish and manage the events you own.</p>
        </div>
        <Button onClick={() => setEditing({})}>+ Create event</Button>
      </div>

      <Alert>{error}</Alert>

      <Panel>
        {loading ? (
          <Spinner />
        ) : events.length === 0 ? (
          <Empty>No events yet — create your first one.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Event</th><th>City</th><th>Dates</th><th>Status</th><th>Regs</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id}>
                  <td><b>{e.name}</b><div className="muted" style={{ fontSize: ".72rem" }}>{e.tagline}</div></td>
                  <td>{e.city}</td>
                  <td>{new Date(e.startDate).toLocaleDateString()}</td>
                  <td><Badge tone={tone[e.status] || "purple"}>{e.status}</Badge></td>
                  <td>{e.registrations}</td>
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      <Button variant="o" size="sm" onClick={() => setManageEvent(e)}>Manage</Button>
                      <Button variant="o" size="sm" onClick={() => setCatEvent(e)}>Categories</Button>
                      <Button variant="o" size="sm" onClick={() => setEditing(e)}>Edit</Button>
                      {e.status === "draft" && (
                        <Button variant="g" size="sm" onClick={() => publish(e)}>Publish</Button>
                      )}
                      <Button variant="danger" size="sm" onClick={() => remove(e)}>Delete</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      {editing && (
        <EventWizard
          editing={editing}
          onClose={() => setEditing(null)}
          onDone={() => { setEditing(null); load(); }}
        />
      )}

      {catEvent && <CategoryManager event={catEvent} onClose={() => setCatEvent(null)} />}
      {manageEvent && <EventManage event={manageEvent} onClose={() => setManageEvent(null)} />}
    </>
  );
}

// CategoryManager is the per-event category editor (add age-banded categories
// from the Ops master data, remove them).
function CategoryManager({ event, onClose }) {
  const [cats, setCats] = useState([]);
  const [refCats, setRefCats] = useState([]);
  const [refBands, setRefBands] = useState([]);
  const [error, setError] = useState("");
  const [form, setForm] = useState({ categoryId: "", ageBandId: "", participationType: "solo", fee: 0 });

  const load = async () => {
    try {
      const [c, rc, rb] = await Promise.all([
        eventApi.eventCategories.list(event.id),
        eventApi.ref.categories(),
        eventApi.ref.ageBands(),
      ]);
      setCats(c || []);
      setRefCats(rc || []);
      setRefBands(rb || []);
    } catch (e) {
      setError(e.message);
    }
  };
  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const add = async () => {
    setError("");
    try {
      await eventApi.eventCategories.add(event.id, form);
      setForm({ categoryId: "", ageBandId: "", participationType: "solo", fee: 0 });
      await load();
    } catch (e) {
      setError(e.message);
    }
  };
  const remove = async (ecId) => {
    try {
      await eventApi.eventCategories.remove(ecId);
      await load();
    } catch (e) {
      setError(e.message);
    }
  };

  return (
    <Modal title={`Categories — ${event.name}`} onClose={onClose}
      footer={<Button variant="o" onClick={onClose}>Done</Button>}>
      <Alert>{error}</Alert>
      {cats.length === 0 ? (
        <Empty>No categories yet. Add one below.</Empty>
      ) : (
        <table>
          <thead><tr><th>Category</th><th>Age band</th><th>Type</th><th>Fee</th><th></th></tr></thead>
          <tbody>
            {cats.map((c) => (
              <tr key={c.id}>
                <td>{c.categoryName}</td>
                <td>{c.ageBandLabel}</td>
                <td>{c.participationType}</td>
                <td>₹{c.fee}</td>
                <td style={{ textAlign: "right" }}>
                  <Button variant="danger" size="sm" onClick={() => remove(c.id)}>Remove</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 14 }}>
        <div className="frow">
          <Field as="select" label="Category" value={form.categoryId}
            onChange={(v) => setForm((f) => ({ ...f, categoryId: v }))}
            options={refCats.map((c) => ({ value: c.id, label: c.label }))} />
          <Field as="select" label="Age band" value={form.ageBandId}
            onChange={(v) => setForm((f) => ({ ...f, ageBandId: v }))}
            options={refBands.map((b) => ({ value: b.id, label: b.label }))} />
          <Field as="select" label="Type" value={form.participationType}
            onChange={(v) => setForm((f) => ({ ...f, participationType: v }))}
            options={["solo", "duet", "group"].map((v) => ({ value: v, label: v }))} />
          <Field label="Fee ₹" type="number" value={form.fee}
            onChange={(v) => setForm((f) => ({ ...f, fee: Number(v) }))} />
        </div>
        <Button onClick={add} disabled={!form.categoryId || !form.ageBandId}>+ Add category</Button>
      </div>
    </Modal>
  );
}
