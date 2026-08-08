import { useEffect, useState } from "react";
import { opsApi } from "../../api/endpoints";
import { Button, Panel, Field, Modal, Badge, Spinner, Alert, Empty, Pagination, usePaged } from "../../components/ui";
import OpsEventManage from "./OpsEventManage";

const tone = { open: "open", live: "live", completed: "draft", draft: "draft" };

// OpsEventsPage — platform-wide event oversight (Admin Design §4.4). Ops can
// edit or delete any event, run the per-event operations hub (crew, expenses,
// P&L, report, archive), but does not create events.
export default function OpsEventsPage() {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(null); // event being edited
  const [form, setForm] = useState({ name: "", city: "", status: "open", fee: 0 });
  const [manage, setManage] = useState(null); // event in the ops hub
  const [busy, setBusy] = useState(false);

  const { pageItems, page, setPage, totalPages } = usePaged(events, 10);

  const load = async () => {
    setLoading(true);
    try {
      setEvents((await opsApi.events.list()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    load();
  }, []);

  const openEdit = (e) => {
    setForm({ name: e.name, city: e.city, status: e.status, fee: e.fee });
    setEditing(e);
  };
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      await opsApi.events.update(editing.id, form);
      setEditing(null);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };
  const remove = async (e) => {
    if (!window.confirm(`Delete "${e.name}" and all its data?`)) return;
    try {
      await opsApi.events.remove(e.id);
      await load();
    } catch (err) {
      setError(err.message);
    }
  };

  if (loading) return <Spinner />;

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>All Events</h1>
          <p>Every event on the platform. Manage crew, expenses, P&amp;L, reports and archiving.</p>
        </div>
      </div>
      <Alert>{error}</Alert>

      <Panel>
        {events.length === 0 ? (
          <Empty>No events on the platform yet.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Event</th><th>City</th><th>Created by</th><th>Status</th><th>Regs</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((e) => (
                <tr key={e.id}>
                  <td><b>{e.name}</b></td>
                  <td>{e.city}</td>
                  <td>{e.createdBy}</td>
                  <td><Badge tone={tone[e.status] || "purple"}>{e.status}</Badge></td>
                  <td>{e.registrations}</td>
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      <Button variant="o" size="sm" onClick={() => setManage(e)}>Manage</Button>
                      <Button variant="o" size="sm" onClick={() => openEdit(e)}>Edit</Button>
                      <Button variant="danger" size="sm" onClick={() => remove(e)}>Delete</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Pagination page={page} totalPages={totalPages} onChange={setPage} />

      {editing && (
        <Modal
          title="Edit event"
          onClose={() => setEditing(null)}
          footer={
            <>
              <Button variant="o" onClick={() => setEditing(null)}>Cancel</Button>
              <Button onClick={save} disabled={busy}>Save</Button>
            </>
          }
        >
          <div className="frow">
            <Field label="Name" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
            <Field label="City" value={form.city} onChange={(v) => setForm((f) => ({ ...f, city: v }))} />
            <Field as="select" label="Status" value={form.status} onChange={(v) => setForm((f) => ({ ...f, status: v }))}
              options={["draft", "open", "live", "completed"].map((v) => ({ value: v, label: v }))} />
            <Field label="Fee ₹" type="number" value={form.fee} onChange={(v) => setForm((f) => ({ ...f, fee: Number(v) }))} />
          </div>
        </Modal>
      )}

      {manage && (
        <OpsEventManage
          event={manage}
          onClose={() => setManage(null)}
          onArchived={() => { setManage(null); load(); }}
        />
      )}
    </>
  );
}
