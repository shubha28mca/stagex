import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Alert, Badge, Button, Modal, Spinner } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

// =============================================================================
// Seat-map designer — Operational Admin
//
// A hall has at most one seating arrangement. Ops lays it out visually here:
// generate blocks of rows, drag seats and the stage/backstage around, rubber-band
// or shift-click a selection, then price a single seat or a whole group.
//
// Everything is edited in local state and committed with ONE save (PUT), which
// is a full replace of the arrangement. That keeps the editor honest: what is on
// the canvas is exactly what is stored. Seats are matched server-side on their
// natural key (section, row, seat number), so seat ids survive a redesign and a
// future booking can safely reference them.
//
// Row geometry comes from the SERVER (`generateRows`) rather than being
// duplicated here, so the layout rule has exactly one implementation.
// =============================================================================

// Rendered size of a seat in canvas units. posX/posY are a seat's CENTRE, which
// keeps group-drag and rubber-band maths symmetric.
const SEAT = 20;
const HALF = SEAT / 2;

// _uid is a client-only identity so selection survives edits to a seat's
// section/row/number (which are its real key, and are editable here).
let uidSeq = 1;
const withUid = (o) => ({ ...o, _uid: `u${uidSeq++}` });

const money = (n) =>
  "₹" + Number(n || 0).toLocaleString("en-IN", { maximumFractionDigits: 0 });

// effectivePrice mirrors the server's rule (and the v_hall_seat_pricing view):
// a seat inherits its tier's price unless it carries an override. Duplicated
// here only so the summary updates live while dragging; the server remains
// authoritative on save.
const effectivePrice = (seat, zoneByCode) =>
  seat.price != null ? seat.price : (zoneByCode.get(seat.zoneCode)?.price ?? 0);

const sellable = (s) => s.isActive && s.kind !== "house";

// Palette offered when adding a price tier.
const TIER_COLORS = ["#6a1b9a", "#ff7a3c", "#37b6ff", "#2ecc8a", "#ff4d8d", "#ffc107", "#0e1237"];

const SEAT_KINDS = [
  { value: "seat", label: "Standard seat" },
  { value: "wheelchair", label: "Wheelchair space" },
  { value: "companion", label: "Companion seat" },
  { value: "house", label: "House / press hold" },
];

// The furniture Ops can drop on the canvas. Defaults are sized for a 1200x800
// canvas so a newly added item lands somewhere sensible and visible.
const ELEMENT_TOOLS = [
  { kind: "stage", label: "Stage", w: 400, h: 80, color: "#0e1237", text: "STAGE" },
  { kind: "backstage", label: "Backstage", w: 300, h: 40, color: "#171c4f", text: "BACKSTAGE" },
  { kind: "aisle", label: "Aisle", w: 24, h: 240, color: "#e9e6f2", text: "" },
  { kind: "entrance", label: "Entrance", w: 130, h: 34, color: "#2ecc8a", text: "ENTRANCE" },
  { kind: "exit", label: "Exit", w: 130, h: 34, color: "#e5484d", text: "EXIT" },
  { kind: "screen", label: "Screen", w: 360, h: 30, color: "#37b6ff", text: "SCREEN" },
  { kind: "label", label: "Text label", w: 140, h: 22, color: "#6b6785", text: "SECTION" },
];

// ---------------------------------------------------------------------------
// Small field helpers, local to the designer's dense tool rail.
// ---------------------------------------------------------------------------
function F({ label, value, onChange, type = "text", ...rest }) {
  return (
    <div className="smf">
      <label>{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(type === "number" ? Number(e.target.value) : e.target.value)}
        {...rest}
      />
    </div>
  );
}
function S({ label, value, onChange, options }) {
  return (
    <div className="smf">
      <label>{label}</label>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </div>
  );
}
function Card({ title, children }) {
  return (
    <div className="smcard">
      <h4>{title}</h4>
      <div className="smbody">{children}</div>
    </div>
  );
}

export default function SeatMapDesigner() {
  const { hallId } = useParams();
  const navigate = useNavigate();

  const [map, setMap] = useState(null);
  const [loading, setLoading] = useState(true);
  const [missing, setMissing] = useState(false); // hall exists but has no arrangement
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  // The arrangement, as edited locally.
  const [name, setName] = useState("Main seating");
  const [canvas, setCanvas] = useState({ w: 1200, h: 800 });
  const [zones, setZones] = useState([]);
  const [seats, setSeats] = useState([]);
  const [elements, setElements] = useState([]);

  // Selection + view.
  const [selected, setSelected] = useState(() => new Set());
  const [selectedEl, setSelectedEl] = useState(null);
  const [marquee, setMarquee] = useState(null);
  const [zoom, setZoom] = useState(1);
  const [grid, setGrid] = useState(2);
  const [showRowGen, setShowRowGen] = useState(false);

  const svgRef = useRef(null);
  const drag = useRef(null);

  const zoneByCode = useMemo(() => new Map(zones.map((z) => [z.code, z])), [zones]);

  // ---- load -------------------------------------------------------------
  const hydrate = useCallback((m) => {
    setMap(m);
    setName(m.name);
    setCanvas({ w: m.canvasWidth, h: m.canvasHeight });
    setZones(m.zones || []);
    setSeats((m.seats || []).map(withUid));
    setElements((m.elements || []).map(withUid));
    setSelected(new Set());
    setSelectedEl(null);
    setDirty(false);
    setMissing(false);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      hydrate(await opsApi.seatMaps.get(hallId));
    } catch (e) {
      // 404 covers both "no such hall" and "hall has no arrangement yet"; only
      // the latter should offer to create one.
      if (e.status === 404 && /no seating arrangement/i.test(e.message)) setMissing(true);
      else setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [hallId, hydrate]);

  useEffect(() => { load(); }, [load]);

  // Warn before losing unsaved layout work on a browser navigation.
  useEffect(() => {
    if (!dirty) return;
    const warn = (e) => { e.preventDefault(); e.returnValue = ""; };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  // ---- derived summary (live, mirrors the server's Summarize) ------------
  const summary = useMemo(() => {
    let total = 0, blocked = 0, house = 0, unpriced = 0, revenue = 0;
    const rows = new Set();
    const perZone = new Map(zones.map((z) => [z.code, { ...z, seats: 0, revenue: 0 }]));
    for (const s of seats) {
      rows.add(`${s.section}|${s.rowLabel}`);
      if (!s.isActive) blocked++;
      if (s.kind === "house") house++;
      if (!sellable(s)) continue;
      const p = effectivePrice(s, zoneByCode);
      if (p <= 0) unpriced++;
      total++;
      revenue += p;
      const z = perZone.get(s.zoneCode);
      if (z) { z.seats++; z.revenue += p; }
    }
    return { total, blocked, house, unpriced, revenue, rows: rows.size, byZone: [...perZone.values()] };
  }, [seats, zones, zoneByCode]);

  // ---- canvas geometry --------------------------------------------------
  const toCanvas = useCallback((e) => {
    const r = svgRef.current.getBoundingClientRect();
    return {
      x: ((e.clientX - r.left) / r.width) * canvas.w,
      y: ((e.clientY - r.top) / r.height) * canvas.h,
    };
  }, [canvas.w, canvas.h]);

  // Snapping the DELTA (not the absolute position) preserves the relative
  // offsets inside a multi-seat selection while still aligning to the grid.
  const snap = useCallback((d) => (grid > 1 ? Math.round(d / grid) * grid : Math.round(d)), [grid]);

  const beginDrag = (e, payload) => {
    drag.current = payload;
    try { svgRef.current.setPointerCapture(e.pointerId); } catch { /* older browsers */ }
  };

  const onSeatDown = (e, seat) => {
    e.stopPropagation();
    setSelectedEl(null);
    const additive = e.shiftKey || e.metaKey || e.ctrlKey;

    if (additive) {
      // Toggle membership; a modified click adjusts the set, it never drags.
      setSelected((prev) => {
        const next = new Set(prev);
        next.has(seat._uid) ? next.delete(seat._uid) : next.add(seat._uid);
        return next;
      });
      return;
    }

    // Dragging an unselected seat selects just it; dragging one already in the
    // selection moves the whole selection.
    const set = selected.has(seat._uid) ? selected : new Set([seat._uid]);
    if (set !== selected) setSelected(set);

    const p = toCanvas(e);
    const base = new Map();
    for (const s of seats) if (set.has(s._uid)) base.set(s._uid, { x: s.posX, y: s.posY });
    beginDrag(e, { mode: "seats", ox: p.x, oy: p.y, base, moved: false });
  };

  const onElDown = (e, el, resize = false) => {
    e.stopPropagation();
    setSelectedEl(el._uid);
    setSelected(new Set());
    const p = toCanvas(e);
    beginDrag(e, {
      mode: resize ? "elresize" : "el",
      ox: p.x, oy: p.y, uid: el._uid,
      base: { x: el.posX, y: el.posY, w: el.width, h: el.height },
    });
  };

  const onBackgroundDown = (e) => {
    const p = toCanvas(e);
    setSelectedEl(null);
    beginDrag(e, { mode: "marquee", ox: p.x, oy: p.y, additive: e.shiftKey, moved: false });
    setMarquee({ x1: p.x, y1: p.y, x2: p.x, y2: p.y });
  };

  const onMove = (e) => {
    const d = drag.current;
    if (!d) return;
    const p = toCanvas(e);
    d.moved = true;

    if (d.mode === "marquee") {
      setMarquee({ x1: d.ox, y1: d.oy, x2: p.x, y2: p.y });
      return;
    }
    const dx = snap(p.x - d.ox);
    const dy = snap(p.y - d.oy);

    if (d.mode === "seats") {
      if (!dx && !dy) return;
      setSeats((prev) => prev.map((s) => {
        const b = d.base.get(s._uid);
        if (!b) return s;
        return {
          ...s,
          posX: Math.max(HALF, Math.min(canvas.w - HALF, b.x + dx)),
          posY: Math.max(HALF, Math.min(canvas.h - HALF, b.y + dy)),
        };
      }));
      setDirty(true);
    } else if (d.mode === "el") {
      setElements((prev) => prev.map((el) => el._uid !== d.uid ? el : {
        ...el,
        posX: Math.max(0, Math.min(canvas.w - el.width, d.base.x + dx)),
        posY: Math.max(0, Math.min(canvas.h - el.height, d.base.y + dy)),
      }));
      setDirty(true);
    } else if (d.mode === "elresize") {
      setElements((prev) => prev.map((el) => el._uid !== d.uid ? el : {
        ...el,
        width: Math.max(12, Math.min(canvas.w - el.posX, d.base.w + dx)),
        height: Math.max(12, Math.min(canvas.h - el.posY, d.base.h + dy)),
      }));
      setDirty(true);
    }
  };

  const onUp = () => {
    const d = drag.current;
    drag.current = null;
    if (!d) return;

    if (d.mode === "marquee") {
      const r = marquee;
      setMarquee(null);
      // A click with no movement on empty canvas means "clear the selection".
      if (!d.moved || !r) { if (!d.additive) setSelected(new Set()); return; }
      const x1 = Math.min(r.x1, r.x2), x2 = Math.max(r.x1, r.x2);
      const y1 = Math.min(r.y1, r.y2), y2 = Math.max(r.y1, r.y2);
      // A seat counts as caught if its box overlaps the band at all.
      const hit = seats
        .filter((s) => s.posX + HALF >= x1 && s.posX - HALF <= x2 &&
                       s.posY + HALF >= y1 && s.posY - HALF <= y2)
        .map((s) => s._uid);
      setSelected((prev) => (d.additive ? new Set([...prev, ...hit]) : new Set(hit)));
    }
  };

  // ---- selection helpers -------------------------------------------------
  const selectedSeats = useMemo(() => seats.filter((s) => selected.has(s._uid)), [seats, selected]);
  const patchSelected = (patch) => {
    if (!selected.size) return;
    setSeats((prev) => prev.map((s) => (selected.has(s._uid) ? { ...s, ...patch } : s)));
    setDirty(true);
  };
  const deleteSelected = () => {
    if (!selected.size) return;
    setSeats((prev) => prev.filter((s) => !selected.has(s._uid)));
    setSelected(new Set());
    setDirty(true);
  };

  // Align a hand-dragged selection onto one row line — the fix-up Ops reaches
  // for constantly after free-dragging seats.
  const alignRow = () => {
    if (selectedSeats.length < 2) return;
    const y = Math.round(selectedSeats.reduce((a, s) => a + s.posY, 0) / selectedSeats.length);
    patchSelected({ posY: y });
  };
  // Space the selection evenly between its own leftmost and rightmost seats.
  const distribute = () => {
    if (selectedSeats.length < 3) return;
    const ordered = [...selectedSeats].sort((a, b) => a.posX - b.posX);
    const first = ordered[0].posX;
    const step = (ordered[ordered.length - 1].posX - first) / (ordered.length - 1);
    const xs = new Map(ordered.map((s, i) => [s._uid, Math.round(first + i * step)]));
    setSeats((prev) => prev.map((s) => (xs.has(s._uid) ? { ...s, posX: xs.get(s._uid) } : s)));
    setDirty(true);
  };
  // Renumber the selection left-to-right, so a hand-built row reads correctly
  // on a ticket even if the seats were placed out of order.
  const renumber = () => {
    if (!selectedSeats.length) return;
    const ordered = [...selectedSeats].sort((a, b) => a.posY - b.posY || a.posX - b.posX);
    const nums = new Map(ordered.map((s, i) => [s._uid, i + 1]));
    setSeats((prev) => prev.map((s) => (nums.has(s._uid)
      ? { ...s, seatNumber: nums.get(s._uid), seatLabel: `${s.rowLabel}${nums.get(s._uid)}` }
      : s)));
    setDirty(true);
  };

  // Keyboard: Delete removes the selection, Escape clears it, Cmd/Ctrl+A takes
  // everything. Ignored while typing in the rail's inputs.
  useEffect(() => {
    const onKey = (e) => {
      const tag = e.target.tagName;
      if (tag === "INPUT" || tag === "SELECT" || tag === "TEXTAREA") return;
      if (e.key === "Escape") { setSelected(new Set()); setSelectedEl(null); }
      else if (e.key === "Delete" || e.key === "Backspace") {
        if (selected.size) { e.preventDefault(); deleteSelected(); }
        else if (selectedEl) {
          e.preventDefault();
          setElements((prev) => prev.filter((el) => el._uid !== selectedEl));
          setSelectedEl(null);
          setDirty(true);
        }
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "a") {
        e.preventDefault();
        setSelected(new Set(seats.map((s) => s._uid)));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seats, selected, selectedEl]);

  // ---- tiers -------------------------------------------------------------
  const addZone = () => {
    const n = zones.length;
    let code = `tier-${n + 1}`;
    while (zones.some((z) => z.code === code)) code = `tier-${Math.random().toString(36).slice(2, 6)}`;
    setZones([...zones, {
      code, name: `Tier ${n + 1}`, color: TIER_COLORS[n % TIER_COLORS.length],
      price: 0, displayOrder: n + 1,
    }]);
    setDirty(true);
  };
  const patchZone = (code, patch) => {
    setZones((prev) => prev.map((z) => (z.code === code ? { ...z, ...patch } : z)));
    setDirty(true);
  };
  const removeZone = (code) => {
    setZones((prev) => prev.filter((z) => z.code !== code));
    // Detach the seats rather than delete them; they become unpriced, which is
    // exactly what blocks publishing until Ops re-tiers them.
    setSeats((prev) => prev.map((s) => (s.zoneCode === code ? { ...s, zoneCode: "" } : s)));
    setDirty(true);
  };

  // ---- elements ----------------------------------------------------------
  const addElement = (tool) => {
    setElements((prev) => [...prev, withUid({
      kind: tool.kind, label: tool.text,
      posX: Math.round((canvas.w - tool.w) / 2),
      posY: 24 + prev.length * 8,
      width: tool.w, height: tool.h, rotation: 0, color: tool.color,
    })]);
    setDirty(true);
  };
  const selEl = elements.find((e) => e._uid === selectedEl) || null;
  const patchEl = (patch) => {
    setElements((prev) => prev.map((el) => (el._uid === selectedEl ? { ...el, ...patch } : el)));
    setDirty(true);
  };

  // ---- row generator -----------------------------------------------------
  // Suggest a starting point that continues the map rather than overlapping it.
  const genDefaults = useMemo(() => {
    const maxRowIdx = seats.reduce((a, s) => Math.max(a, s.rowIndex), -1);
    const maxY = seats.reduce((a, s) => Math.max(a, s.posY), 0);
    const sections = [...new Set(seats.map((s) => s.section))];
    return {
      section: sections[0] || "Orchestra",
      rows: 6, seatsPerRow: 20,
      firstRowIndex: maxRowIdx + 1,
      firstRowLetter: maxRowIdx + 1,
      numberFrom: 1,
      zoneCode: zones[0]?.code || "",
      kind: "seat",
      centerX: Math.round(canvas.w / 2),
      originY: seats.length ? Math.min(canvas.h - 40, maxY + 40) : 180,
      seatPitch: 26, rowPitch: 30, curve: 6,
    };
  }, [seats, zones, canvas.w, canvas.h]);

  const generate = async (spec) => {
    setBusy(true);
    setError("");
    try {
      const made = await opsApi.seatMaps.generateRows(spec);
      // Never let a generated block silently overwrite existing seats — they
      // share a natural key, so the server would upsert over them.
      const taken = new Set(seats.map((s) => `${s.section}|${s.rowLabel}|${s.seatNumber}`));
      const fresh = made.filter((s) => !taken.has(`${s.section}|${s.rowLabel}|${s.seatNumber}`));
      const clashes = made.length - fresh.length;
      setSeats((prev) => [...prev, ...fresh.map(withUid)]);
      setSelected(new Set());
      setDirty(true);
      setShowRowGen(false);
      setNotice(
        `Added ${fresh.length} seats` +
        (clashes ? ` — skipped ${clashes} that already exist in ${spec.section}` : "")
      );
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- save --------------------------------------------------------------
  // The payload is built field by field: the API rejects unknown fields, and
  // the loaded map carries read-only extras (id, effectivePrice, _uid).
  const payload = (status) => ({
    name,
    canvasWidth: canvas.w,
    canvasHeight: canvas.h,
    status,
    zones: zones.map((z) => ({
      code: z.code, name: z.name, color: z.color,
      price: Number(z.price) || 0, displayOrder: Number(z.displayOrder) || 0,
    })),
    seats: seats.map((s) => ({
      section: s.section, rowLabel: s.rowLabel, rowIndex: Number(s.rowIndex) || 0,
      seatNumber: Number(s.seatNumber), seatLabel: s.seatLabel || `${s.rowLabel}${s.seatNumber}`,
      zoneCode: s.zoneCode || "",
      price: s.price == null ? null : Number(s.price),
      kind: s.kind || "seat",
      posX: Math.round(s.posX), posY: Math.round(s.posY),
      isActive: s.isActive !== false,
    })),
    elements: elements.map((e) => ({
      kind: e.kind, label: e.label || "",
      posX: Math.round(e.posX), posY: Math.round(e.posY),
      width: Math.round(e.width), height: Math.round(e.height),
      rotation: Number(e.rotation) || 0, color: e.color,
    })),
  });

  const save = async (status) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      hydrate(await opsApi.seatMaps.save(hallId, payload(status)));
      setNotice(status === "published" ? "Arrangement published." : "Arrangement saved.");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const createMap = async () => {
    setBusy(true);
    setError("");
    try {
      hydrate(await opsApi.seatMaps.create(hallId));
      setNotice("Arrangement created — add rows to begin.");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  // ---- render ------------------------------------------------------------
  if (loading) return <Spinner />;

  if (missing) {
    return (
      <>
        <div className="ptitle">
          <div>
            <h1>Seating arrangement</h1>
            <p>This hall does not have a seating arrangement yet.</p>
          </div>
          <Button variant="o" onClick={() => navigate("/ops/seat-maps")}>← Back</Button>
        </div>
        <Alert>{error}</Alert>
        <div className="smcard">
          <div className="smbody" style={{ alignItems: "flex-start", gap: 14 }}>
            <p className="smhint" style={{ maxWidth: 560, fontSize: ".82rem" }}>
              Creating an arrangement gives you a canvas with a stage and backstage
              already placed. Everything is measured from the stage — a row's
              distance from it is stored with every seat, so tickets and the
              client seat picker always agree on what “front row” means.
            </p>
            <Button onClick={createMap} disabled={busy}>
              {busy ? "Creating…" : "Create seating arrangement"}
            </Button>
          </div>
        </div>
      </>
    );
  }

  if (!map) return <Alert>{error || "Could not load this hall."}</Alert>;

  const published = map.status === "published";

  return (
    <>
      <div className="ptitle">
        <div>
          <h1>{map.hallName} — seating</h1>
          <p>
            {map.hallCity} · listed capacity {map.hallCapacity} ·{" "}
            {published ? <Badge tone="open">published</Badge> : <Badge tone="draft">draft</Badge>}{" "}
            {dirty && <Badge tone="review">unsaved changes</Badge>}
          </p>
        </div>
        <div className="smbar">
          <Button variant="o" size="sm" onClick={() => navigate("/ops/seat-maps")}>← All halls</Button>
          <Button variant="o" onClick={() => save("draft")} disabled={busy}>
            {busy ? "Saving…" : "Save draft"}
          </Button>
          <Button variant="g" onClick={() => save("published")} disabled={busy}>
            {published ? "Save & republish" : "Publish"}
          </Button>
        </div>
      </div>

      <Alert>{error}</Alert>
      {notice && <Alert tone="ok">{notice}</Alert>}

      <div className="smwrap">
        {/* ---------------- tool rail ---------------- */}
        <div className="smcol">
          <Card title="Seats">
            <Button size="sm" onClick={() => setShowRowGen(true)}>+ Add rows of seats</Button>
            <p className="smhint">
              Generates a block of rows, centred and gently curved like a real
              house. Row distance from the stage is set automatically.
            </p>
          </Card>

          <Card title="Stage & furniture">
            {ELEMENT_TOOLS.map((t) => (
              <Button key={t.kind} variant="o" size="sm" onClick={() => addElement(t)}>
                + {t.label}
              </Button>
            ))}
          </Card>

          <Card title="Canvas">
            <div className="smf2">
              <F label="Width" type="number" value={canvas.w}
                 onChange={(v) => { setCanvas((c) => ({ ...c, w: v })); setDirty(true); }} />
              <F label="Height" type="number" value={canvas.h}
                 onChange={(v) => { setCanvas((c) => ({ ...c, h: v })); setDirty(true); }} />
            </div>
            <F label="Arrangement name" value={name}
               onChange={(v) => { setName(v); setDirty(true); }} />
            <div className="smf2">
              <S label="Snap grid" value={String(grid)} onChange={(v) => setGrid(Number(v))}
                 options={[{ value: "1", label: "Off" }, { value: "2", label: "2 units" },
                           { value: "13", label: "Half seat" }, { value: "26", label: "One seat" }]} />
              <S label="Zoom" value={String(zoom)} onChange={(v) => setZoom(Number(v))}
                 options={[{ value: "0.6", label: "60%" }, { value: "0.8", label: "80%" },
                           { value: "1", label: "100%" }, { value: "1.4", label: "140%" },
                           { value: "2", label: "200%" }]} />
            </div>
          </Card>

          <Card title="How to use">
            <p className="smhint">
              <b>Click</b> a seat to select · <b>Shift-click</b> to add or remove ·
              <b> drag on empty space</b> to rubber-band a group · <b>drag a seat</b> to
              move the whole selection · <b>Delete</b> removes it · <b>Esc</b> clears.
            </p>
          </Card>
        </div>

        {/* ---------------- canvas ---------------- */}
        <div className="smcol">
          <div className="smstage">
            <svg
              ref={svgRef}
              width={canvas.w * zoom}
              height={canvas.h * zoom}
              viewBox={`0 0 ${canvas.w} ${canvas.h}`}
              onPointerDown={onBackgroundDown}
              onPointerMove={onMove}
              onPointerUp={onUp}
              onPointerCancel={onUp}
            >
              {/* furniture sits under the seats */}
              {elements.map((el) => {
                const isSel = el._uid === selectedEl;
                const light = el.kind === "aisle" || el.kind === "label";
                return (
                  <g key={el._uid} className="smel" onPointerDown={(e) => onElDown(e, el)}>
                    <rect
                      x={el.posX} y={el.posY} width={el.width} height={el.height}
                      rx={el.kind === "stage" ? 10 : 6}
                      fill={el.color}
                      opacity={light ? 0.55 : 1}
                      stroke={isSel ? "var(--gold)" : "none"}
                      strokeWidth={isSel ? 3 : 0}
                    />
                    {el.label && (
                      <text
                        x={el.posX + el.width / 2} y={el.posY + el.height / 2}
                        textAnchor="middle" dominantBaseline="central"
                        fontSize={Math.max(9, Math.min(16, el.height * 0.42))}
                        fontFamily="Poppins, sans-serif" fontWeight="700"
                        letterSpacing="1.5"
                        fill={light ? "#4a4560" : "#fff"}
                        pointerEvents="none"
                      >
                        {el.label}
                      </text>
                    )}
                    {isSel && (
                      // Bottom-right resize grip.
                      <rect
                        x={el.posX + el.width - 7} y={el.posY + el.height - 7}
                        width={14} height={14} rx={3}
                        fill="var(--gold)" style={{ cursor: "nwse-resize" }}
                        onPointerDown={(e) => onElDown(e, el, true)}
                      />
                    )}
                  </g>
                );
              })}

              {/* seats */}
              {seats.map((s) => {
                const zone = zoneByCode.get(s.zoneCode);
                const isSel = selected.has(s._uid);
                const p = effectivePrice(s, zoneByCode);
                const blocked = !s.isActive;
                const fill = blocked ? "#d9d6e3" : (zone?.color || "#c9c4d8");
                return (
                  <g key={s._uid} className="smseat" onPointerDown={(e) => onSeatDown(e, s)}>
                    <title>
                      {`${s.section} · row ${s.rowLabel} (${s.rowIndex} from stage) · seat ${s.seatNumber}`}
                      {`\n${blocked ? "BLOCKED" : s.kind === "house" ? "HOUSE HOLD" : money(p)}`}
                      {s.price != null && !blocked ? " (per-seat override)" : ""}
                      {!zone && !blocked && s.kind !== "house" ? "\nNo price tier assigned" : ""}
                    </title>
                    <rect
                      x={s.posX - HALF} y={s.posY - HALF}
                      width={SEAT} height={SEAT}
                      rx={s.kind === "wheelchair" ? 3 : 6}
                      fill={fill}
                      opacity={blocked ? 0.6 : 1}
                      stroke={isSel ? "var(--navy)" : (s.price != null ? "var(--gold)" : "rgba(0,0,0,.14)")}
                      strokeWidth={isSel ? 3 : (s.price != null ? 2 : 1)}
                    />
                    {/* A glyph beats a colour for the non-standard kinds. */}
                    {s.kind === "wheelchair" && (
                      <text x={s.posX} y={s.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="11" fill="#fff" pointerEvents="none">♿</text>
                    )}
                    {s.kind === "house" && (
                      <text x={s.posX} y={s.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="10" fontWeight="700" fill="#fff" pointerEvents="none">H</text>
                    )}
                    {blocked && (
                      <text x={s.posX} y={s.posY} textAnchor="middle" dominantBaseline="central"
                            fontSize="11" fontWeight="700" fill="#8a8598" pointerEvents="none">×</text>
                    )}
                  </g>
                );
              })}

              {marquee && (
                <rect
                  className="smmarquee"
                  x={Math.min(marquee.x1, marquee.x2)} y={Math.min(marquee.y1, marquee.y2)}
                  width={Math.abs(marquee.x2 - marquee.x1)} height={Math.abs(marquee.y2 - marquee.y1)}
                  pointerEvents="none"
                />
              )}
            </svg>
          </div>

          <div className="smcard">
            <div className="smbody">
              <div className="smlegend">
                {zones.map((z) => (
                  <i key={z.code}><s style={{ background: z.color }} />{z.name} · {money(z.price)}</i>
                ))}
                <i><s style={{ background: "#c9c4d8" }} />No tier</i>
                <i><s style={{ background: "#d9d6e3" }} />Blocked</i>
                <i><s style={{ background: "#fff", border: "2px solid var(--gold)" }} />Per-seat price</i>
              </div>
            </div>
          </div>
        </div>

        {/* ---------------- inspector ---------------- */}
        <div className="smcol">
          <Card title="Arrangement">
            <div className="smstats">
              <div className="smstat"><b>{summary.total}</b><span>SELLABLE SEATS</span></div>
              <div className="smstat"><b>{summary.rows}</b><span>ROWS</span></div>
              <div className={`smstat ${summary.unpriced ? "warn" : ""}`}>
                <b>{summary.unpriced}</b><span>UNPRICED</span>
              </div>
              <div className="smstat"><b>{summary.blocked + summary.house}</b><span>BLOCKED / HELD</span></div>
            </div>
            <div className="smstat"><b>{money(summary.revenue)}</b><span>FULL-HOUSE VALUE</span></div>
            {summary.unpriced > 0 && (
              <p className="smhint" style={{ color: "var(--red)" }}>
                {summary.unpriced} sellable seat{summary.unpriced === 1 ? "" : "s"} have no
                price. Publishing is blocked until every one has a tier or its own price.
              </p>
            )}
          </Card>

          <Card title="Price tiers">
            {zones.length === 0 && <p className="smhint">No tiers yet. Add one, then select seats to assign it.</p>}
            {zones.map((z) => (
              <div className="smzone" key={z.code}>
                <input className="sw" type="color" value={z.color} title="Tier colour"
                       onChange={(e) => patchZone(z.code, { color: e.target.value })} />
                <input className="nm" type="text" value={z.name} title={`code: ${z.code}`}
                       onChange={(e) => patchZone(z.code, { name: e.target.value })} />
                <div className="pr">
                  <input type="number" min="0" step="10" value={z.price}
                         onChange={(e) => patchZone(z.code, { price: Number(e.target.value) })} />
                </div>
                <button className="x" title="Remove tier" onClick={() => removeZone(z.code)}>✕</button>
              </div>
            ))}
            <Button variant="o" size="sm" onClick={addZone}>+ Add price tier</Button>
            <div className="smhint">
              {summary.byZone.filter((z) => z.seats).map((z) => (
                <div key={z.code}>{z.name}: {z.seats} seats · {money(z.revenue)}</div>
              ))}
            </div>
          </Card>

          <SelectionPanel
            seats={selectedSeats}
            zones={zones}
            onPatch={patchSelected}
            onDelete={deleteSelected}
            onAlign={alignRow}
            onDistribute={distribute}
            onRenumber={renumber}
            money={money}
            zoneByCode={zoneByCode}
          />

          {selEl && (
            <Card title={`Selected: ${selEl.kind}`}>
              <F label="Label" value={selEl.label} onChange={(v) => patchEl({ label: v })} />
              <div className="smf2">
                <F label="X" type="number" value={selEl.posX} onChange={(v) => patchEl({ posX: v })} />
                <F label="Y" type="number" value={selEl.posY} onChange={(v) => patchEl({ posY: v })} />
              </div>
              <div className="smf2">
                <F label="Width" type="number" value={selEl.width} onChange={(v) => patchEl({ width: v })} />
                <F label="Height" type="number" value={selEl.height} onChange={(v) => patchEl({ height: v })} />
              </div>
              <div className="smf">
                <label>Colour</label>
                <input type="color" value={selEl.color} onChange={(e) => patchEl({ color: e.target.value })} />
              </div>
              <Button variant="danger" size="sm" onClick={() => {
                setElements((prev) => prev.filter((el) => el._uid !== selectedEl));
                setSelectedEl(null);
                setDirty(true);
              }}>Remove {selEl.kind}</Button>
            </Card>
          )}
        </div>
      </div>

      {showRowGen && (
        <RowGeneratorModal
          defaults={genDefaults}
          zones={zones}
          busy={busy}
          onClose={() => setShowRowGen(false)}
          onGenerate={generate}
        />
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Inspector panel for the current seat selection — the "price one seat or a
// whole group" surface.
// ---------------------------------------------------------------------------
function SelectionPanel({ seats, zones, onPatch, onDelete, onAlign, onDistribute, onRenumber, money, zoneByCode }) {
  const [priceInput, setPriceInput] = useState("");

  // A typed price belongs to the selection it was typed for — clear it when the
  // selection changes so an old figure can't be applied to different seats.
  const signature = seats.map((s) => s._uid).join(",");
  useEffect(() => { setPriceInput(""); }, [signature]);

  const n = seats.length;
  if (!n) {
    return (
      <Card title="Selection">
        <p className="smhint">
          Nothing selected. Click a seat, shift-click to build up a group, or drag
          across empty canvas to rubber-band a whole block — then set a price for
          all of them at once.
        </p>
      </Card>
    );
  }

  // Summarize what is selected so Ops can confirm before applying a price.
  const one = n === 1 ? seats[0] : null;
  const rows = [...new Set(seats.map((s) => `${s.section} ${s.rowLabel}`))];
  const total = seats.reduce(
    (a, s) => a + (s.price != null ? s.price : (zoneByCode.get(s.zoneCode)?.price ?? 0)), 0);

  // The dropdowns show the selection's shared value, or blank when it is mixed
  // — derived rather than held in state, so they can never go stale.
  const shared = (key) => (seats.every((s) => s[key] === seats[0][key]) ? seats[0][key] : "");
  const sharedZone = shared("zoneCode");
  const sharedKind = shared("kind");

  return (
    <Card title={one ? `Seat ${one.seatLabel}` : `${n} seats selected`}>
      {one ? (
        <p className="smhint">
          {one.section} · row {one.rowLabel} · seat {one.seatNumber}<br />
          <b>{one.rowIndex}</b> row{one.rowIndex === 1 ? "" : "s"} from the stage
        </p>
      ) : (
        <p className="smhint">
          {rows.length} row{rows.length === 1 ? "" : "s"}: {rows.slice(0, 4).join(", ")}
          {rows.length > 4 ? ` +${rows.length - 4} more` : ""}<br />
          Current total: <b>{money(total)}</b>
        </p>
      )}

      <div className="smf">
        <label>Assign price tier</label>
        <select value={sharedZone} onChange={(e) => onPatch({ zoneCode: e.target.value })}>
          <option value="">{sharedZone === "" && n > 1 ? "— mixed / no tier —" : "— no tier —"}</option>
          {zones.map((z) => (
            <option key={z.code} value={z.code}>{z.name} · {money(z.price)}</option>
          ))}
        </select>
      </div>

      <div className="smf">
        <label>Per-seat price override (₹)</label>
        <div className="smbar">
          <input
            className="grow" type="number" min="0" step="10" value={priceInput}
            placeholder="e.g. 1800"
            onChange={(e) => setPriceInput(e.target.value)}
            style={{ border: "1.5px solid var(--line)", borderRadius: 9, padding: "8px 10px", fontFamily: "Inter", fontSize: ".84rem" }}
          />
          <Button size="sm" disabled={priceInput === ""}
                  onClick={() => onPatch({ price: Number(priceInput) })}>Apply</Button>
        </div>
        <p className="smhint">
          Overrides the tier for {n === 1 ? "this seat" : "these seats"} only —
          use it for restricted views or comps.
        </p>
        <Button variant="o" size="sm" onClick={() => { setPriceInput(""); onPatch({ price: null }); }}>
          Clear override (inherit tier)
        </Button>
      </div>

      <div className="smf">
        <label>Seat type</label>
        <select value={sharedKind} onChange={(e) => onPatch({ kind: e.target.value })}>
          {sharedKind === "" && <option value="">— mixed —</option>}
          {SEAT_KINDS.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
        </select>
      </div>

      <div className="smbar">
        <Button variant="o" size="sm" onClick={() => onPatch({ isActive: false })}>Block</Button>
        <Button variant="o" size="sm" onClick={() => onPatch({ isActive: true })}>Unblock</Button>
      </div>

      <div className="smbar">
        <Button variant="o" size="sm" onClick={onAlign} disabled={n < 2}>Align row</Button>
        <Button variant="o" size="sm" onClick={onDistribute} disabled={n < 3}>Space evenly</Button>
      </div>
      <Button variant="o" size="sm" onClick={onRenumber}>Renumber left → right</Button>
      <Button variant="danger" size="sm" onClick={onDelete}>Delete {n} seat{n === 1 ? "" : "s"}</Button>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Row-block generator. The geometry itself is computed by the API so the editor
// and the stored map can never disagree about where a row sits.
// ---------------------------------------------------------------------------
function RowGeneratorModal({ defaults, zones, busy, onClose, onGenerate }) {
  const [f, setF] = useState(defaults);
  const set = (k) => (v) => setF((p) => ({ ...p, [k]: v }));

  return (
    <Modal
      title="Add rows of seats"
      onClose={onClose}
      footer={
        <>
          <Button variant="o" onClick={onClose}>Cancel</Button>
          <Button onClick={() => onGenerate(f)} disabled={busy}>
            {busy ? "Adding…" : `Add ${(f.rows || 0) * (f.seatsPerRow || 0)} seats`}
          </Button>
        </>
      }
    >
      <div className="frow">
        <F label="Section name" value={f.section} onChange={set("section")} />
        <S label="Price tier" value={f.zoneCode} onChange={set("zoneCode")}
           options={[{ value: "", label: "— no tier —" },
                     ...zones.map((z) => ({ value: z.code, label: z.name }))]} />
      </div>
      <div className="frow">
        <F label="Number of rows" type="number" min="1" max="100" value={f.rows} onChange={set("rows")} />
        <F label="Seats per row" type="number" min="1" max="200" value={f.seatsPerRow} onChange={set("seatsPerRow")} />
      </div>
      <div className="frow">
        <F label="First row's distance from stage" type="number" min="0"
           value={f.firstRowIndex} onChange={set("firstRowIndex")} />
        <F label="First row letter (0=A, 8=J)" type="number" min="0"
           value={f.firstRowLetter} onChange={set("firstRowLetter")} />
      </div>
      <div className="frow">
        <F label="Number seats from" type="number" min="1" value={f.numberFrom} onChange={set("numberFrom")} />
        <S label="Seat type" value={f.kind} onChange={set("kind")} options={SEAT_KINDS} />
      </div>
      <div className="frow">
        <F label="Centre X on canvas" type="number" value={f.centerX} onChange={set("centerX")} />
        <F label="First row Y on canvas" type="number" value={f.originY} onChange={set("originY")} />
      </div>
      <div className="frow">
        <F label="Seat spacing" type="number" min="1" value={f.seatPitch} onChange={set("seatPitch")} />
        <F label="Row spacing" type="number" min="1" value={f.rowPitch} onChange={set("rowPitch")} />
      </div>
      <div className="frow one">
        <F label="Row curve (0 = straight)" type="number" min="0" value={f.curve} onChange={set("curve")} />
      </div>
      <p className="smhint">
        Row letters skip <b>I</b> and <b>O</b> — they read as 1 and 0 on a printed
        ticket. Distance from the stage is stored per seat, so a balcony can
        restart its lettering while still being 13 rows back.
      </p>
    </Modal>
  );
}
