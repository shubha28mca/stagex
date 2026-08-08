import { useEffect, useState } from "react";
import { peopleApi } from "../api/endpoints";
import { Button, Panel, Field, Spinner, Alert, Empty, Badge, initials, Pagination, usePaged } from "../components";

// MyPeoplePage — list, add, edit and delete the family's people.
// - Add/edit share one form; identity fields (gender, Aadhaar) are set only at
//   creation and are read-only afterwards (ClientDesignWeb §5.1 / §7).
// - Deleting a person still attached to an incomplete event soft-removes them:
//   they stay listed, grayed-out, until that event completes.
export default function MyPeoplePage() {
  const [people, setPeople] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [ok, setOk] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState(null); // null → add mode

  const empty = { name: "", dob: "", gender: "", aadhaar: "", relationship: "", school: "", city: "", guru: "" };
  const [form, setForm] = useState(empty);
  const [busy, setBusy] = useState(false);

  const { pageItems, page, setPage, totalPages } = usePaged(people, 8);

  const load = async () => {
    setLoading(true);
    try {
      setPeople((await peopleApi.list()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));

  const openAdd = () => {
    setEditingId(null);
    setForm(empty);
    setShowForm(true);
    setError("");
    setOk("");
  };

  const openEdit = (p) => {
    setEditingId(p.id);
    setForm({
      name: p.name,
      dob: (p.dob || "").slice(0, 10), // ISO timestamp → YYYY-MM-DD for the date input
      gender: p.gender,
      aadhaar: "",
      relationship: p.relationship || "",
      school: p.school || "",
      city: p.city || "",
      guru: p.guru || "",
    });
    setShowForm(true);
    setError("");
    setOk("");
  };

  const submit = async () => {
    setError("");
    setOk("");
    setBusy(true);
    try {
      if (editingId) {
        // Only editable fields are sent; gender/Aadhaar are omitted on purpose.
        await peopleApi.update(editingId, {
          name: form.name,
          dob: form.dob,
          relationship: form.relationship,
          school: form.school,
          city: form.city,
          guru: form.guru,
        });
        setOk(`${form.name} updated.`);
      } else {
        await peopleApi.create(form);
        setOk(`${form.name} added.`);
      }
      setForm(empty);
      setEditingId(null);
      setShowForm(false);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (p) => {
    if (!window.confirm(`Remove ${p.name}? If they are in an event that hasn't finished, they'll be kept (grayed out) until it completes.`)) {
      return;
    }
    setError("");
    setOk("");
    try {
      const res = await peopleApi.remove(p.id);
      setOk(res.message || "Removed.");
      await load();
    } catch (e) {
      setError(e.message);
    }
  };

  return (
    <>
      <div className="row spread mb">
        <div>
          <h1 style={{ color: "var(--navy)", fontSize: "1.45rem" }}>My People</h1>
          <p className="muted">Family members you can register across events.</p>
        </div>
        <Button variant="g" onClick={() => (showForm && !editingId ? setShowForm(false) : openAdd())}>
          {showForm && !editingId ? "Close" : "+ Add person"}
        </Button>
      </div>

      <Alert>{error}</Alert>
      <Alert tone="ok">{ok}</Alert>

      {showForm && (
        <Panel title={editingId ? "Edit person" : "Add a person"}>
          <p className="muted mb" style={{ fontSize: ".8rem" }}>
            {editingId
              ? "Update details below. Gender and Aadhaar are set at creation and can't be changed here."
              : "Mandatory: name, date of birth, gender, Aadhaar. School / city / guru are optional."}
          </p>
          <div className="grid2">
            <Field label="Full name" value={form.name} onChange={set("name")} />
            <Field label="Date of birth" type="date" value={form.dob} onChange={set("dob")} />
            {!editingId && (
              <Field
                as="select"
                label="Gender"
                value={form.gender}
                onChange={set("gender")}
                options={[
                  { value: "female", label: "Female" },
                  { value: "male", label: "Male" },
                  { value: "other", label: "Other" },
                ]}
              />
            )}
            {!editingId && (
              <Field label="Aadhaar (12 digits)" value={form.aadhaar} onChange={set("aadhaar")} inputMode="numeric" maxLength={12} />
            )}
            <Field
              as="select"
              label="Relationship"
              value={form.relationship}
              onChange={set("relationship")}
              options={["Daughter", "Son", "Niece", "Nephew", "Myself", "Student", "Other"].map((r) => ({ value: r, label: r }))}
            />
            <Field label="School (optional)" value={form.school} onChange={set("school")} />
            <Field label="City (optional)" value={form.city} onChange={set("city")} />
            <Field label="Guru name (optional)" value={form.guru} onChange={set("guru")} />
          </div>
          <div className="row">
            <Button variant="g" onClick={submit} disabled={busy}>
              {editingId ? "Save changes" : "Save person"}
            </Button>
            <Button variant="o" onClick={() => { setShowForm(false); setEditingId(null); }}>
              Cancel
            </Button>
          </div>
        </Panel>
      )}

      {loading ? (
        <Spinner />
      ) : people.length === 0 ? (
        <Empty>No people yet — add your first family member above.</Empty>
      ) : (
        <>
          {pageItems.map((p) => (
            <div className="pcard" key={p.id} style={{ cursor: "default", opacity: p.deleted ? 0.5 : 1 }}>
              <span className="pavatar" style={p.deleted ? { filter: "grayscale(1)" } : undefined}>
                {initials(p.name)}
              </span>
              <div style={{ flex: 1 }}>
                <div className="row" style={{ gap: 8 }}>
                  <b style={p.deleted ? { textDecoration: "line-through" } : undefined}>{p.name}</b>
                  {p.deleted && <Badge tone="err">Removed · retained until event completes</Badge>}
                </div>
                <div className="muted" style={{ fontSize: ".78rem" }}>
                  Age {p.ageYears} · {p.relationship} · Aadhaar {p.aadhaarMasked}
                  {p.city ? ` · ${p.city}` : ""}
                </div>
              </div>
              {!p.deleted && (
                <div className="row" style={{ gap: 6 }}>
                  <Button variant="o" size="sm" onClick={() => openEdit(p)}>Edit</Button>
                  <Button variant="o" size="sm" onClick={() => remove(p)}>Delete</Button>
                </div>
              )}
            </div>
          ))}
          <Pagination page={page} totalPages={totalPages} onChange={setPage} />
        </>
      )}
    </>
  );
}
