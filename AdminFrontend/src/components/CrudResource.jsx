import { useEffect, useMemo, useState } from "react";
import { Button, Panel, Field, Modal, Spinner, Alert, Empty, Pagination, usePaged } from "./ui";

// CrudResource is the reusable engine behind every master-data screen. Give it
// an api ({list, create, update, remove}), the table `columns` and the form
// `fields`, and it renders a full list + add/edit modal + delete flow. This is
// what keeps the seven Operational-Admin master pages to a few lines each.
//
// field: { key, label, type: "text"|"number"|"select"|"checkbox", options?, hidden? }
// column: { key, label, render?(row) }
export default function CrudResource({ title, subtitle, api, columns, fields, canDelete = true, canCreate = true }) {
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(null); // row being edited, {} for new, null closed
  const [form, setForm] = useState({});
  const [busy, setBusy] = useState(false);

  const { pageItems, page, setPage, totalPages } = usePaged(rows, 10);

  const defaults = useMemo(() => {
    const d = {};
    fields.forEach((f) => (d[f.key] = f.type === "checkbox" ? true : f.type === "number" ? 0 : ""));
    return d;
  }, [fields]);

  const load = async () => {
    setLoading(true);
    try {
      setRows((await api.list()) || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const openAdd = () => {
    setForm(defaults);
    setEditing({});
    setError("");
  };
  const openEdit = (row) => {
    const f = { ...defaults };
    fields.forEach((fl) => (f[fl.key] = row[fl.key] ?? f[fl.key]));
    setForm(f);
    setEditing(row);
    setError("");
  };

  const set = (key, type) => (v) =>
    setForm((f) => ({ ...f, [key]: type === "number" ? Number(v) : v }));

  const save = async () => {
    setError("");
    setBusy(true);
    try {
      if (editing.id) await api.update(editing.id, form);
      else await api.create(form);
      setEditing(null);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (row) => {
    if (!window.confirm(`Delete this ${title.replace(/s$/, "").toLowerCase()}?`)) return;
    setError("");
    try {
      await api.remove(row.id);
      await load();
    } catch (e) {
      setError(e.message);
    }
  };

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>{title}</h1>
          {subtitle && <p>{subtitle}</p>}
        </div>
        {canCreate && <Button onClick={openAdd}>+ New</Button>}
      </div>

      <Alert>{error}</Alert>

      <Panel>
        {loading ? (
          <Spinner />
        ) : rows.length === 0 ? (
          <Empty>Nothing here yet — click “New” to add the first one.</Empty>
        ) : (
          <table>
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.key}>{c.label}</th>
                ))}
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((row) => (
                <tr key={row.id}>
                  {columns.map((c) => (
                    <td key={c.key}>{c.render ? c.render(row) : String(row[c.key] ?? "")}</td>
                  ))}
                  <td>
                    <div className="rowact" style={{ justifyContent: "flex-end" }}>
                      <Button variant="o" size="sm" onClick={() => openEdit(row)}>Edit</Button>
                      {canDelete && (
                        <Button variant="danger" size="sm" onClick={() => remove(row)}>Delete</Button>
                      )}
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
          title={editing.id ? `Edit ${title.replace(/s$/, "")}` : `New ${title.replace(/s$/, "")}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <Button variant="o" onClick={() => setEditing(null)}>Cancel</Button>
              <Button onClick={save} disabled={busy}>{editing.id ? "Save" : "Create"}</Button>
            </>
          }
        >
          <Alert>{error}</Alert>
          <div className="frow">
            {fields
              .filter((f) => !f.hidden || !editing.id)
              .map((f) =>
                f.type === "checkbox" ? (
                  <label key={f.key} className="row" style={{ gap: 8, gridColumn: "1 / -1" }}>
                    <input
                      type="checkbox"
                      checked={!!form[f.key]}
                      onChange={(e) => setForm((s) => ({ ...s, [f.key]: e.target.checked }))}
                    />
                    {f.label}
                  </label>
                ) : f.type === "select" ? (
                  <Field
                    key={f.key}
                    as="select"
                    label={f.label}
                    value={form[f.key] ?? ""}
                    onChange={set(f.key)}
                    options={f.options}
                  />
                ) : (
                  <Field
                    key={f.key}
                    label={f.label}
                    type={f.type === "number" ? "number" : "text"}
                    value={form[f.key] ?? ""}
                    onChange={set(f.key, f.type)}
                    disabled={editing.id && f.lockOnEdit}
                  />
                )
              )}
          </div>
        </Modal>
      )}
    </>
  );
}
