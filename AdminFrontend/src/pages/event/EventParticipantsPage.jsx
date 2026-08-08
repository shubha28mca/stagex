import { useEffect, useState } from "react";
import { eventApi } from "../../api/endpoints";
import { Button, Panel, Field, Modal, Badge, Spinner, Alert, Empty, Pagination, usePaged } from "../../components/ui";

const payTone = { paid: "open", pending: "draft", held: "review" };

// EventParticipantsPage — participants registered in the admin's own events.
// Event Admins may edit a participant's editable profile fields (§5.4).
export default function EventParticipantsPage() {
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState({ name: "", city: "", school: "", guru: "" });
  const [busy, setBusy] = useState(false);

  const { pageItems, page, setPage, totalPages } = usePaged(rows, 10);

  const load = async () => {
    setLoading(true);
    try {
      setRows((await eventApi.participants.list()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    load();
  }, []);

  const openEdit = (p) => {
    setForm({ name: p.name, city: "", school: "", guru: "" });
    setEditing(p);
    setError("");
  };
  const save = async () => {
    setError("");
    setBusy(true);
    try {
      await eventApi.participants.update(editing.personId, form);
      setEditing(null);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Spinner />;

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>Participants</h1>
          <p>Everyone registered across your events.</p>
        </div>
      </div>
      <Alert>{error}</Alert>

      <Panel>
        {rows.length === 0 ? (
          <Empty>No participants yet — publish an event and share it.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Participant</th><th>Event</th><th>Category</th><th>Entry</th>
                <th>Payment</th><th>Family</th><th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((p, i) => (
                <tr key={`${p.personId}-${i}`}>
                  <td><b>{p.name}</b></td>
                  <td>{p.eventName}</td>
                  <td>{p.categoryName}</td>
                  <td>#{p.entryCode}</td>
                  <td><Badge tone={payTone[p.payStatus] || "draft"}>{p.payStatus}</Badge></td>
                  <td>{p.familyPhone}</td>
                  <td style={{ textAlign: "right" }}>
                    <Button variant="o" size="sm" onClick={() => openEdit(p)}>Edit</Button>
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
          title={`Edit ${editing.name}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <Button variant="o" onClick={() => setEditing(null)}>Cancel</Button>
              <Button onClick={save} disabled={busy}>Save</Button>
            </>
          }
        >
          <Alert>{error}</Alert>
          <div className="frow">
            <Field label="Name" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
            <Field label="City" value={form.city} onChange={(v) => setForm((f) => ({ ...f, city: v }))} />
            <Field label="School" value={form.school} onChange={(v) => setForm((f) => ({ ...f, school: v }))} />
            <Field label="Guru" value={form.guru} onChange={(v) => setForm((f) => ({ ...f, guru: v }))} />
          </div>
        </Modal>
      )}
    </>
  );
}
