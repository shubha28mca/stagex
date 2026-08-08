import { useEffect, useMemo, useState } from "react";
import { eventApi } from "../../api/endpoints";
import { Button, Field, Modal, Alert, Badge } from "../../components/ui";

// EventWizard is the multi-stage create/edit flow for an event:
//   Event Details → Rounds → Judging rubric → Fees & capacity → Age groups → Review.
// Age groups (category × age band) are what participants match against at
// registration, so they are defined here as part of creating the event.
const STEPS = ["Event Details", "Rounds", "Judging rubric", "Fees & capacity", "Age groups", "Review"];

const blank = {
  name: "", tagline: "", city: "", mode: "onstage", coverGradient: "purple",
  startDate: "", endDate: "", fee: 0, slotsTotal: 0,
  roundsDetail: [{ name: "Preliminary", description: "" }],
  rubric: [{ criterion: "Technique", weight: 50 }],
  judgeIds: [],
  categories: [], // [{ ecId?, categoryId, ageBandId, participationType, fee }]
};

export default function EventWizard({ editing, onClose, onDone }) {
  const isEdit = !!editing?.id;
  const [step, setStep] = useState(0);
  const [form, setForm] = useState(blank);
  const [judges, setJudges] = useState([]);
  const [refCats, setRefCats] = useState([]);
  const [refBands, setRefBands] = useState([]);
  const [originalEcIds, setOriginalEcIds] = useState([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    Promise.all([eventApi.ref.judges(), eventApi.ref.categories(), eventApi.ref.ageBands()])
      .then(([j, rc, rb]) => { setJudges(j || []); setRefCats(rc || []); setRefBands(rb || []); })
      .catch(() => {});
    if (isEdit) {
      setForm({
        name: editing.name, tagline: editing.tagline || "", city: editing.city,
        mode: editing.mode, coverGradient: editing.coverGradient,
        startDate: (editing.startDate || "").slice(0, 10),
        endDate: (editing.endDate || "").slice(0, 10),
        fee: editing.fee, slotsTotal: editing.slotsTotal,
        roundsDetail: editing.roundsDetail?.length ? editing.roundsDetail : blank.roundsDetail,
        rubric: editing.rubric?.length ? editing.rubric : blank.rubric,
        judgeIds: editing.judgeIds || [],
        categories: [],
      });
      // Load the event's existing age groups so they can be updated in place.
      eventApi.eventCategories.list(editing.id).then((cats) => {
        const rows = (cats || []).map((c) => ({
          ecId: c.id, categoryId: c.categoryId, ageBandId: c.ageBandId,
          participationType: c.participationType, fee: c.fee,
        }));
        setOriginalEcIds(rows.map((r) => r.ecId));
        setForm((f) => ({ ...f, categories: rows }));
      }).catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const set = (k, num) => (v) => setForm((f) => ({ ...f, [k]: num ? Number(v) : v }));
  const rubricTotal = useMemo(() => form.rubric.reduce((s, c) => s + (Number(c.weight) || 0), 0), [form.rubric]);

  // ---- dynamic list helpers ----
  const addRound = () => setForm((f) => ({ ...f, roundsDetail: [...f.roundsDetail, { name: "", description: "" }] }));
  const setRound = (i, k, v) =>
    setForm((f) => ({ ...f, roundsDetail: f.roundsDetail.map((r, j) => (j === i ? { ...r, [k]: v } : r)) }));
  const delRound = (i) => setForm((f) => ({ ...f, roundsDetail: f.roundsDetail.filter((_, j) => j !== i) }));

  const addCrit = () => setForm((f) => ({ ...f, rubric: [...f.rubric, { criterion: "", weight: 0 }] }));
  const setCrit = (i, k, v) =>
    setForm((f) => ({ ...f, rubric: f.rubric.map((c, j) => (j === i ? { ...c, [k]: k === "weight" ? Number(v) : v } : c)) }));
  const delCrit = (i) => setForm((f) => ({ ...f, rubric: f.rubric.filter((_, j) => j !== i) }));

  const toggleJudge = (id) =>
    setForm((f) => ({ ...f, judgeIds: f.judgeIds.includes(id) ? f.judgeIds.filter((x) => x !== id) : [...f.judgeIds, id] }));

  const addCat = () =>
    setForm((f) => ({ ...f, categories: [...f.categories, { categoryId: "", ageBandId: "", participationType: "solo", fee: f.fee || 0 }] }));
  const setCat = (i, k, v) =>
    setForm((f) => ({ ...f, categories: f.categories.map((c, j) => (j === i ? { ...c, [k]: k === "fee" ? Number(v) : v } : c)) }));
  const delCat = (i) => setForm((f) => ({ ...f, categories: f.categories.filter((_, j) => j !== i) }));

  // ---- validation per step ----
  const stepError = () => {
    if (step === 0) {
      if (!form.name || !form.city) return "Name and city are required.";
      if (!form.startDate || !form.endDate) return "Start and end dates are required.";
      if (form.endDate < form.startDate) return "End date cannot be before the start date.";
    }
    if (step === 1 && form.roundsDetail.some((r) => !r.name)) return "Give every round a name (or remove it).";
    return "";
  };

  const next = () => {
    const e = stepError();
    if (e) { setError(e); return; }
    setError("");
    setStep((s) => Math.min(s + 1, STEPS.length - 1));
  };
  const back = () => { setError(""); setStep((s) => Math.max(s - 1, 0)); };

  const submit = async (publish) => {
    setError("");
    setBusy(true);
    try {
      // Only the fields the backend accepts (categories are synced separately).
      const payload = {
        name: form.name, tagline: form.tagline, city: form.city, mode: form.mode,
        coverGradient: form.coverGradient, startDate: form.startDate, endDate: form.endDate,
        fee: form.fee, slotsTotal: form.slotsTotal, rounds: form.roundsDetail.length || 1,
        roundsDetail: form.roundsDetail, rubric: form.rubric, judgeIds: form.judgeIds,
      };
      let id = editing?.id;
      if (isEdit) await eventApi.events.update(id, payload);
      else id = (await eventApi.events.create(payload)).id;

      // Reconcile age groups: add new rows, delete removed ones.
      const rows = form.categories.filter((r) => r.categoryId && r.ageBandId);
      const currentEcIds = rows.filter((r) => r.ecId).map((r) => r.ecId);
      for (const ecId of originalEcIds.filter((x) => !currentEcIds.includes(x))) {
        await eventApi.eventCategories.remove(ecId);
      }
      for (const r of rows.filter((r) => !r.ecId)) {
        await eventApi.eventCategories.add(id, {
          categoryId: r.categoryId, ageBandId: r.ageBandId,
          participationType: r.participationType || "solo", fee: Number(r.fee) || 0,
        });
      }

      if (publish) await eventApi.events.publish(id);
      onDone();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const footer =
    step < STEPS.length - 1 ? (
      <>
        {step > 0 && <Button variant="o" onClick={back}>Back</Button>}
        <Button onClick={next}>Next</Button>
      </>
    ) : (
      <>
        <Button variant="o" onClick={back}>Back</Button>
        <Button variant="o" onClick={() => submit(false)} disabled={busy}>Save as draft</Button>
        <Button variant="g" onClick={() => submit(true)} disabled={busy}>Publish now</Button>
      </>
    );

  return (
    <Modal title={isEdit ? "Edit event" : "Create event"} onClose={onClose} footer={footer}>
      {/* step indicator */}
      <div className="row" style={{ gap: 6, marginBottom: 16 }}>
        {STEPS.map((label, i) => (
          <span key={label} className={`chip ${i === step ? "on" : ""}`} style={{ flex: 1, textAlign: "center", fontSize: ".68rem" }}>
            {i + 1}. {label}
          </span>
        ))}
      </div>

      <Alert>{error}</Alert>

      {step === 0 && (
        <div className="frow">
          <Field label="Event name" value={form.name} onChange={set("name")} />
          <Field label="Tagline" value={form.tagline} onChange={set("tagline")} />
          <Field label="City" value={form.city} onChange={set("city")} />
          <Field as="select" label="Mode" value={form.mode} onChange={set("mode")}
            options={[{ value: "onstage", label: "On-stage" }, { value: "online", label: "Online" }]} />
          <Field as="select" label="Cover colour" value={form.coverGradient} onChange={set("coverGradient")}
            options={["purple", "orange", "pink", "sky"].map((v) => ({ value: v, label: v }))} />
          <div />
          <Field label="Start date" type="date" value={form.startDate} onChange={set("startDate")} />
          <Field label="End date" type="date" value={form.endDate} onChange={set("endDate")} />
        </div>
      )}

      {step === 1 && (
        <div>
          <p className="muted mb" style={{ fontSize: ".82rem" }}>Define each round participants will go through.</p>
          {form.roundsDetail.map((r, i) => (
            <div className="row" key={i} style={{ gap: 8, marginBottom: 8, alignItems: "flex-start" }}>
              <div style={{ flex: 1 }}>
                <Field label={`Round ${i + 1} name`} value={r.name} onChange={(v) => setRound(i, "name", v)} />
              </div>
              <div style={{ flex: 2 }}>
                <Field label="Description" value={r.description} onChange={(v) => setRound(i, "description", v)} />
              </div>
              <Button variant="danger" size="sm" onClick={() => delRound(i)} style={{ marginTop: 6 }}>✕</Button>
            </div>
          ))}
          <Button variant="o" size="sm" onClick={addRound}>+ Add round</Button>
        </div>
      )}

      {step === 2 && (
        <div>
          <div className="row spread mb">
            <p className="muted" style={{ fontSize: ".82rem" }}>Scoring criteria and their weights.</p>
            <Badge tone={rubricTotal === 100 ? "open" : "review"}>weight total: {rubricTotal}%</Badge>
          </div>
          {form.rubric.map((c, i) => (
            <div className="row" key={i} style={{ gap: 8, marginBottom: 8 }}>
              <div style={{ flex: 2 }}>
                <Field label={`Criterion ${i + 1}`} value={c.criterion} onChange={(v) => setCrit(i, "criterion", v)} />
              </div>
              <div style={{ width: 120 }}>
                <Field label="Weight %" type="number" value={c.weight} onChange={(v) => setCrit(i, "weight", v)} />
              </div>
              <Button variant="danger" size="sm" onClick={() => delCrit(i)} style={{ marginTop: 6 }}>✕</Button>
            </div>
          ))}
          <Button variant="o" size="sm" onClick={addCrit}>+ Add criterion</Button>

          <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
            <p className="muted mb" style={{ fontSize: ".82rem" }}>Assign judges from the Ops-verified pool (optional).</p>
            {judges.length === 0 ? (
              <p className="muted" style={{ fontSize: ".78rem" }}>No verified judges available.</p>
            ) : (
              judges.map((j) => (
                <label key={j.id} className="row" style={{ gap: 8, padding: "4px 0" }}>
                  <input type="checkbox" checked={form.judgeIds.includes(j.id)} onChange={() => toggleJudge(j.id)} />
                  {j.label}
                </label>
              ))
            )}
          </div>
        </div>
      )}

      {step === 3 && (
        <div className="frow">
          <Field label="Base entry fee ₹" type="number" value={form.fee} onChange={set("fee", true)} />
          <Field label="Total capacity (slots)" type="number" value={form.slotsTotal} onChange={set("slotsTotal", true)} />
          <p className="muted" style={{ gridColumn: "1 / -1", fontSize: ".78rem" }}>
            This is the default fee; each age group can set its own fee in the next step.
          </p>
        </div>
      )}

      {step === 4 && (
        <div>
          <div className="row spread mb">
            <p className="muted" style={{ fontSize: ".82rem" }}>
              Age groups are what participants register into — a category paired with an age band. A person is
              only eligible if their age falls inside the band.
            </p>
            <Badge tone={form.categories.length ? "open" : "review"}>{form.categories.length} group(s)</Badge>
          </div>
          {form.categories.length === 0 && (
            <p className="muted mb" style={{ fontSize: ".8rem" }}>
              No age groups yet — add at least one, otherwise no participant can register for this event.
            </p>
          )}
          {form.categories.map((c, i) => (
            <div className="row" key={i} style={{ gap: 8, marginBottom: 8, alignItems: "flex-start" }}>
              <div style={{ flex: 2 }}>
                <Field as="select" label="Category" value={c.categoryId} onChange={(v) => setCat(i, "categoryId", v)}
                  options={refCats.map((x) => ({ value: x.id, label: x.label }))} />
              </div>
              <div style={{ flex: 2 }}>
                <Field as="select" label="Age group" value={c.ageBandId} onChange={(v) => setCat(i, "ageBandId", v)}
                  options={refBands.map((x) => ({ value: x.id, label: x.label }))} />
              </div>
              <div style={{ width: 108 }}>
                <Field as="select" label="Type" value={c.participationType} onChange={(v) => setCat(i, "participationType", v)}
                  options={["solo", "duet", "group"].map((v) => ({ value: v, label: v }))} />
              </div>
              <div style={{ width: 96 }}>
                <Field label="Fee ₹" type="number" value={c.fee} onChange={(v) => setCat(i, "fee", v)} />
              </div>
              <Button variant="danger" size="sm" onClick={() => delCat(i)} style={{ marginTop: 6 }}>✕</Button>
            </div>
          ))}
          <Button variant="o" size="sm" onClick={addCat}>+ Add age group</Button>
        </div>
      )}

      {step === 5 && (
        <div>
          <Review label="Event" value={`${form.name} · ${form.city} · ${form.mode}`} />
          <Review label="Dates" value={`${form.startDate} → ${form.endDate}`} />
          <Review label="Rounds" value={form.roundsDetail.map((r) => r.name).join(", ") || "—"} />
          <Review label="Rubric" value={form.rubric.map((c) => `${c.criterion} (${c.weight}%)`).join(", ") || "—"} />
          <Review label="Judges" value={form.judgeIds.length ? `${form.judgeIds.length} assigned` : "none"} />
          <Review label="Age groups" value={form.categories.length
            ? form.categories.map((c) => {
                const cat = refCats.find((x) => x.id === c.categoryId)?.label || "category";
                const band = refBands.find((x) => x.id === c.ageBandId)?.label || "age band";
                return `${cat} · ${band} (₹${c.fee})`;
              }).join(", ")
            : "none — participants can’t register yet"} />
          <Review label="Fee / capacity" value={`₹${form.fee} · ${form.slotsTotal} slots`} />
          <p className="muted mt" style={{ fontSize: ".8rem" }}>
            Save as a draft to keep editing, or publish now to open public registration.
          </p>
        </div>
      )}
    </Modal>
  );
}

function Review({ label, value }) {
  return (
    <div className="row spread" style={{ padding: "8px 0", borderBottom: "1px solid var(--line)" }}>
      <span className="muted" style={{ fontSize: ".8rem" }}>{label}</span>
      <b style={{ fontSize: ".85rem", textAlign: "right" }}>{value}</b>
    </div>
  );
}
