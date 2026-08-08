# IIG StageX — Admin Console Design Document

**Product:** IIG StageX — Performing Arts Championship & Talent Discovery Platform
**Document type:** Design review for Leadership (Admin flows: Super Admin, Operation Admin, Event Admin)
**Companion artifact:** `stagex-admin-wireframes.html` — interactive, clickable wireframes (18 screens)
**Version:** 2.1 · July 2026
**Supersedes:** Admin wireframes v1.0 · Extends PRD v1.0 (§6 RBAC, §8 modules) · v2.1 adds Hall Master (registry, leads, booking process & propagation)

---

## 1. How to review this document

- Every flow below maps to a **live screen in the interactive wireframe file** — open `stagex-admin-wireframes.html` in any browser, no setup needed.
- Use the **role switcher in the top bar** (Event Admin / Operations Admin / Super Admin) to see how the console changes per role.
- Each section here gives: purpose → granular capabilities → key fields & validations → business rules → the wireframe screen to demo.
- A 5-minute **demo script** for leadership walk-throughs is at the end (§8).

### 1.1 Screen map (wireframe navigation)

| # | Screen | Role | Left-nav label |
|---|---|---|---|
| 1 | Dashboard | Event Admin | Dashboard |
| 2 | My Events (own + shared) | Event Admin | My Events |
| 3 | Create Event — 6-step wizard | Event Admin | Create Event |
| 4 | Participants + edit profile | Event Admin | Participants |
| 5 | Notifications (triggers + ad-hoc) | Event Admin | Notifications |
| 6 | Reports & Export | Event Admin | Reports & Export |
| 7 | Ops Dashboard | Operations Admin | Ops Dashboard |
| 8 | Crews & Volunteers (+ roster) | Operations Admin | Crews & Volunteers |
| 9 | Hall Booking Approvals | Operations Admin | Hall Booking Approvals |
| 10 | Vendors | Operations Admin | Vendors |
| 11 | All Events oversight + assign admins | Operations Admin | All Events |
| 12 | Judges pool | Operations Admin | Judges |
| 13 | Event Types & Taxonomy | Operations Admin | Event Types & Taxonomy |
| 14 | Payment Links & Coupons | Operations Admin | Payment Links & Coupons |
| 15 | Sponsor Profiles | Operations Admin | Sponsors |
| 16 | Reports & P&L | Operations Admin | Reports & P&L |
| 17 | Roles & Permissions matrix | Super Admin | Roles & Permissions |
| 18 | Create Sub Admin | Super Admin | Create Sub Admin |

---

## 2. Admin hierarchy — the model in one picture

```
                        ┌──────────────────┐
                        │   SUPER ADMIN    │  owns the platform
                        │  (IIG HQ)        │  creates sub-admins,
                        └────────┬─────────┘  sets permissions
                     creates & scopes
              ┌──────────────────┴──────────────────┐
              ▼                                      ▼
   ┌─────────────────────┐              ┌──────────────────────┐
   │  OPERATION ADMIN    │              │    EVENT ADMIN       │
   │  platform setup &   │              │  event lifecycle     │
   │  operations         │              │  owner               │
   ├─────────────────────┤              ├──────────────────────┤
   │ • Crews + rosters   │              │ • Create events      │
   │ • Volunteers        │   supplies   │ • Publish for public │
   │ • Hall booking      │   masters &  │ • Modify venue/date  │
   │   approvals         │──approvals──▶│ • Participants +     │
   │ • Vendors           │              │   edit profiles      │
   │ • Judges pool       │              │ • Notifications:     │
   │ • Event types +     │              │   triggers + ad-hoc  │
   │   taxonomy          │              │ • Reports & export   │
   │ • Payment links     │              │ • Offline payments   │
   │ • Coupon codes      │              │   verification       │
   │ • Sponsor profiles  │   oversees   │                      │
   │ • Sees ALL events + │◀──(read)─────│ sees ONLY own events │
   │   creators; assigns │              │ + events shared with │
   │   Event Admins      │              │ them                 │
   │ • Ops reports & P&L │              │                      │
   └─────────────────────┘              └──────────────────────┘
```

**The one-line separation:** *Event Admins own the content of their events; Operation Admins own the shared machinery every event runs on (people, venues, money instruments, masters) plus platform-wide oversight.* Super Admin owns who gets which powers.

---

## 3. Super Admin flow

### 3.1 Purpose
Create and govern sub-admins. The Super Admin does not run events day-to-day; they mint the Operation Admins and Event Admins who do, scope their access, and audit everything.

### 3.2 Create Sub Admin — flow (wireframe: **Create Sub Admin**, screen 18)

```
┌─ Create Sub Admin ───────────────────────────────┬─ Existing sub-admins ─┐
│ Full name*        │ Mobile (OTP invite)*         │ Priya K  Event  Active│
│ Email* [validate] │ Scope: region/org*           │ Arjun M  Event  Active│
│ Role template*    │ Access expiry (optional)     │ Rohit S  Ops    Active│
│  ▸ Operation Admin / Event Admin / Custom        │ Lata N   Ops    Invite│
│ ── Permissions (auto-filled, editable) ─────────  ├───────────────────────┤
│  [on] Approve/reject hall bookings               │ Audit trail           │
│  [on] Crews, rosters & volunteers                │ • Role granted…       │
│  [on] Judges, taxonomy, links, coupons, sponsors │ • Permission revoked… │
│  [on] Ops reports & P&L                          │                       │
│  [off] Assign Event Admins to events             │                       │
│  [off] Create/maintain events                    │                       │
│           [Cancel]  [Create & send invite]       │                       │
└──────────────────────────────────────────────────┴───────────────────────┘
```

- **Inputs:** full name*, mobile* (OTP-verified invite), email* (format-validated — a live error state is shown in the wireframe), scope* (region/organization), role template*, optional access expiry.
- **Role templates:** picking *Operation Admin* or *Event Admin* auto-fills the permission set from the matrix (§6); Super Admin can then toggle individual grants — e.g., an "Approvals-only" Ops sub-admin (Lata N in the wireframe).
- **Business rules:**
  - Invitee activates via OTP-verified link and sets their own password; account shows "Invite sent" until then.
  - Scope restricts data visibility (SX-FR-004): a South-region Ops Admin sees only South-region bookings, crews and reports.
  - Every grant, edit and revocation is audit-logged with before/after values (SX-FR-006).
  - Access expiry auto-revokes on date; revocation is immediate and session-killing.
  - One person can hold multiple roles (SX-FR-002) — templates stack, conflicts resolve to the more permissive grant with a warning.
- **Governance screen** (wireframe: **Roles & Permissions**, screen 17): the full capability matrix across the three roles, with locked defaults (purple) and grantable exceptions (grey), the admin roster, and the audit log.

---

## 4. Operation Admin — full capability set

The Operation Admin is the platform's operations backbone. Ten capability areas, each demoable:

### 4.1 Crews, rosters & volunteers (screen 8)
- Create crews with: name*, event*, function* (Stage/Registration/Green Room/AV/Security/Hospitality), crew lead, team size*, shift timing.
- Assign the **roster**: add members from the volunteer pool (228 onboarded in the demo data) by search or bulk import; members receive SMS/WhatsApp invites with shift and venue details on save.
- Track readiness: staffing bars per crew, understaffed flags, lead-unassigned alerts, QR check-in points for attendance.
- Rules: a crew belongs to exactly one event; a volunteer can serve multiple crews only if shifts don't overlap (conflict warning); attendance feeds the volunteer attendance report (§4.10).

### 4.2 Hall master & booking approvals (screen 9)

The Hall Booking Approvals screen now has three parts: the **approval queue**, the **hall registry (master data)**, and the **Add Hall** form.

**4.2.1 Add a hall (master data)**
- **Inputs:** hall name*, city*, seating capacity*, base rate (₹/day), **hall lead** (name* + phone/email*), alternate lead, advance % required, blackout dates, minimum lead time (days before event), and required documents from the Event Admin (layout plan, safety/fire NOC, GST details, insurance…).
- **Booking process builder:** an **ordered, versioned list of steps**, each with an owner. Default template:
  1. Availability auto-check — *System (instant)*
  2. Document verification — *Hall lead*
  3. Advance payment (e.g., 40%) — *Event Admin, via an Ops payment link (§4.8)*
  4. Ops final approval — *Ops Admin*
  Steps can be added, removed and reordered per hall; each hall carries its own process version (v1, v2, v3…).
- **On save:** the hall becomes selectable in the Event Admin's "Preferred venue" dropdown (Create Event, step 1); the hall lead receives an invite and is notified at every step they own; new booking requests follow the hall's current process version.

**4.2.2 Hall leads**
- Every hall has a named lead (and optional alternate) who owns specific process steps — e.g., document verification. The lead is shown on each booking request's detail panel and timeline, so the Event Admin always knows who is holding a step.
- A hall without a lead is flagged ("lead required") and cannot accept new bookings until one is assigned.

**4.2.3 Propagation rule — "update once, re-flow everywhere"**
When Ops publishes a change to a hall's **process** or **calendar** (rates, blackout dates, lead time, steps):
- Every **pending** booking for that hall is automatically re-run: steps are **re-sequenced** to the new process version, requested **dates are revalidated** against the new calendar, and conflicts are re-checked.
- Affected **Event Admins are notified automatically** with what changed and what (if anything) they must redo (e.g., a newly added document).
- **Approved** bookings are never silently changed: they are untouched unless a date now conflicts (e.g., a new blackout), in which case they are **flagged for Ops review** with the Event Admin copied.
- Each booking records which process version it was approved under, for audit.

**4.2.4 Approvals queue (unchanged mechanics, now process-aware)**
- Queue of requests raised on event publish; detail panel shows event, rounds, venue, capacity vs footfall, quotation, documents, the automatic conflict check, and now the **hall's process version and current step with its owner** (e.g., "v3 · step 2 of 4 — document verification, hall lead R. Naik").
- Decisions: **Approve** (unlocks the event's stage schedule), **Reject** (note mandatory), or **Propose alternative**; SLA badges and a status timeline per request.

### 4.3 Vendors (screen 10)
- Vendor registry: service type, city, assigned events, contract status, payment stage (advance/full), rating.
- New vendor requests (e.g., #VN-118 LED wall) land in the same approval discipline as hall bookings.
- Vendor payments and dues roll up into event P&L (§4.10).

### 4.4 All-events oversight — who created what (screen 11)
- A table of **every event on the platform** with a **"Created by" column** (which Event Admin owns it) plus current co-admins, dates and status — the visibility Event Admins themselves don't have.
- Ops has **read-only** access to event content: they can view, not edit — event content stays the Event Admin's domain.

### 4.5 Assign Event Admins to existing events (screen 11, lower panel)
- Pick an event → add one or more Event Admins → choose access level: **Co-administrator** (full manage, cannot delete the event), **Results only**, or **Broadcast only**.
- On assignment: owner and added admins are notified; the event appears in the assignee's "My Events" under **Shared with me**.
- Ownership transfer is a separate, confirm-twice action reserved for Ops/Super Admin.

### 4.6 Judges & judge profiles (screen 12)
- Platform-wide judge pool that Event Admins draw from when building panels.
- Add judge: name*, mobile*, email, primary expertise*, years of experience*, affiliated school/academy (drives **conflict-of-interest** enforcement — SX-FR-163), bio, photo, ID proof, credentials.
- Verification workflow: profiles are "ID check pending" until Ops verifies; only verified judges are assignable.
- Pool shows events judged, participant-facing rating, and expertise filters.

### 4.7 Event types, categories & sub-categories (screen 13)
- **Event types** master (National Championship, Regional, School, Online-only) — each carries default rules and certificate branding (gold seal + hologram, silver seal, standard, digital badge).
- **Taxonomy editor** for the 11-category tree (PRD §7): add/edit/archive categories and sub-categories; each node carries default rubric and media rules.
- Rule: taxonomy is **versioned** — published events keep the version they launched with; archiving hides a node from *new* events only. Custom sub-categories proposed by organizers require Ops approval (SX-FR-012).

### 4.8 Payment links (screen 14)
- Create links per event and purpose: **Entry fee / Sponsor invoice / Merchandise / Donation**; amount, validity, and allowed modes (UPI, cards, net-banking, **offline reference code**).
- Each link shows collections and usage count and reconciles automatically into the event P&L.
- Offline-mode links generate reference codes; the **Event Admin** verifies receipt on their Participants screen (§5.4) — creation is Ops, verification is Event Admin.

### 4.9 Coupon codes & sponsor profiles (screens 14–15)
- Coupons: code*, type (% / flat ₹ / 100% sponsored), value*, scope (global / single event / category), max uses, validity. Created here → appear in the Event Admin's "attach coupon" picker in the Create Event wizard.
- Sponsors: organisation*, tier* (Platinum/Gold/Silver/Impact), contact*, committed amount, **funded scholarship slots**, branding placements (stage banner, certificates, LED, app splash, stream overlay), logo and agreement uploads. Sponsored-slot usage is tracked against commitment.

### 4.10 Operational reports & P&L (screen 16)
- Platform KPIs: total revenue (fees + sponsorship), total expenses (venue + vendors + crew), **net P&L and margin**, offline collections pending verification.
- **P&L by event** table — revenue and expense breakdown per event with its owning Event Admin, including loss-making flags (Rangoli Prix shows −21% in the demo data to illustrate).
- Report catalogue with CSV/XLSX/PDF export: registrations, settlement & reconciliation (gateway vs offline vs refunds), vendor payments & dues, volunteer attendance, hall utilisation. Every figure drills to source transactions.

---

## 5. Event Admin — full capability set

### 5.1 Event visibility rule (screen 2)
- An Event Admin sees **only** (a) events they created, and (b) events **shared with them** (by another Event Admin via Ops assignment, §4.5). The wireframe shows "Garba Fiesta 2026 — *Shared by Arjun M*" with a "Shared with me" filter chip and an explanatory banner.
- No Event Admin can browse or open another admin's unshared events; the Ops oversight view (§4.4) is the only cross-admin lens.

### 5.2 Create, publish & maintain events (screen 3)
- The 6-step wizard: Details → Categories & age bands (from the Ops-managed taxonomy) → Rounds & schedule → Judging rubric (judges picked from the Ops-managed pool) → Fees & capacity → Review & **Publish**.
- **Publish** opens public registration immediately; if an on-stage round's hall booking is still pending Ops approval, the stage schedule stays hidden until approved (shown on the Review step).
- **Modify after publish:** venue and date remain editable; any change to a published event's schedule/venue **automatically fires the "Schedule change" notification** to all affected participants (see §5.5) and re-triggers hall booking if the venue changed. All edits are audit-logged.
- **Payment configuration (step 5):** attach Ops-created coupon codes, set sponsored-slot count, and select accepted modes — including **offline modes: cash at venue, bank transfer, cheque/DD**.

### 5.3 Dashboard & reports export (screens 1 and 6)
- Dashboard: registrations, revenue, judging progress, upcoming deadlines, "needs attention" items.
- **Reports & Export** screen: dashboard summary (PDF/XLSX), full participant list, revenue & reconciliation (online vs offline vs coupons vs sponsored), judging progress, results & rankings, certificates issued — plus a **scheduled weekly email** of the dashboard summary to leadership. All exports are audit-logged.

### 5.4 Participants — list, edit, offline verification (screen 4)
- Filterable roster per event: participant, guardian, entry ID, category · age band · type, payment status, check-in.
- **Edit profile** panel: name, DOB, category/entry, guardian contact — with a **mandatory "reason for edit"** field; every change is audit-logged and the guardian is notified. DOB edits re-validate age-band eligibility; a band change flags the entry for review.
- **Offline payments:** entries paid by cash/transfer/cheque sit in "Offline — pending verification"; the Event Admin clicks **"Mark offline payment received"** to confirm the entry. Pending offline totals surface on both the Event Admin's and Ops' report screens.

### 5.5 Notifications — configured & ad-hoc (screen 5)
- **Automatic triggers** per event, each with per-channel toggles (in-app / email / SMS / WhatsApp): registration confirmed, payment received, schedule/venue change, round reminder (24 h), results published. Templates editable with merge tags.
- **Ad-hoc broadcast composer:** audience segments (all participants / by category / round qualifiers / payment pending), channel selection, message with merge tags (`{name} {event} {round} {entry_id}`), live preview, send now or schedule. Sends are rate-limited and logged; scoped strictly to the admin's own/shared events.

---

## 6. Updated permission matrix (source of truth — screen 17)

| Capability | Event Admin | Operation Admin | Super Admin |
|---|:---:|:---:|:---:|
| Create / edit / publish events | ● | — | ● |
| Configure categories, rounds, rubric, fees | ● | — | ● |
| Publish results & leaderboards | ● | — | ● |
| Request hall / venue booking | ● | — | ● |
| **Approve / reject hall bookings** | — | ● | ● |
| Create crews, rosters & volunteers | — | ● | ● |
| Manage vendors & contracts | — | ● | ● |
| Venue master data (add halls, leads & booking process) | — | ● | ● |
| **Add judges & manage judge profiles** | — | ● | ● |
| **Manage event types & taxonomy** | — | ● | ● |
| **Create payment links & coupon codes** | — | ● | ● |
| **Create & manage sponsor profiles** | — | ● | ● |
| **See all events & assign Event Admins** | — | ● | ● |
| **Operational reports & event P&L** | — | ● | ● |
| View & edit participants (own/shared events) | ● | — | ● |
| Configure & send event notifications | ● | — | ● |
| Record / verify offline payments | ● | — | ● |
| Platform finance & refunds oversight | — | — | ● |
| **Create sub-admins & set permissions** | — | — | ● |
| Edit this permission matrix | — | — | ● |

● = locked default from this design; per-user exceptions are grantable by the Super Admin and audit-logged.

### 6.1 Changes since PRD v1.0 (for leadership sign-off)
1. **Sub-admin creation formalized** — Super Admin mints Ops/Event Admins with templated, editable permissions, scope and expiry (extends SX-FR-005).
2. **Judges moved to Ops** — the judge pool (profiles, verification, conflict flags) is Ops-owned; Event Admins assign from the pool (refines SX-FR-134).
3. **Taxonomy & event types moved to Ops** — previously Super Admin (SX-FR-220); now Ops with versioning rules.
4. **Payment links & coupons are Ops-created** — Event Admins attach, not create (refines SX-FR-133/152).
5. **Sponsor profiles Ops-created** — sponsor dashboards unchanged (SX-FR-190).
6. **Event sharing introduced** — multiple Event Admins per event via Ops assignment; "own + shared" visibility rule.
7. **Offline payment modes added** — cash/transfer/cheque with Event Admin verification (extends SX-FR-150/153).
8. **Ops P&L reporting added** — event-level profit & loss (extends §14 of the PRD).
9. **Hall master introduced (v2.1)** — Ops adds halls with named leads and a versioned, per-hall booking process; publishing hall updates re-flows all pending bookings (steps + dates) with automatic notification, while approved bookings are only flagged on conflict (extends SX-FR-200).

---

## 7. Cross-role workflows (the seams that must work)

**W1 · Hall booking:** Event Admin publishes with preferred venue → system raises booking request + auto conflict-check → Ops approves/rejects/proposes → on approval the stage schedule unlocks and participants can see it; on rejection the Event Admin edits venue/date, which re-fires the cycle and the schedule-change notification.

**W2 · Event sharing:** Ops (or Super) assigns Event Admin B to Event Admin A's event → B sees it under "Shared with me" with the granted access level → both can manage per their level; all actions are attributed individually in the audit log.

**W3 · Money flow:** Ops creates payment link/coupon → Event Admin attaches in wizard step 5 → participants pay (online instantly confirmed; offline pending) → Event Admin verifies offline receipts → everything reconciles into Ops P&L and the Event Admin's revenue report.

**W4 · Judge panel:** Ops adds & verifies judge (with affiliation) → Event Admin assigns to rubric/round → system blocks the judge from scoring entries from their affiliated school → scores roll up to results.

**W5 · Hall master update propagation:** Ops edits a hall (new process step, rate, blackout dates) → publish creates a new process version → all **pending** bookings for that hall re-sequence to the new steps and revalidate dates → affected Event Admins are notified with required actions → approved bookings are only flagged (never auto-changed) if a new conflict arises → each booking permanently records its approved process version.

---

## 8. Five-minute leadership demo script

1. **Open the wireframe → Super Admin** → *Create Sub Admin*: show role template auto-filling permissions, toggle one off, "Create & send invite." (45 s)
2. Still Super Admin → *Roles & Permissions*: point at the locked split between the two admin types. (30 s)
3. **Switch to Event Admin** → *Create Event*: click through the 6 steps; on step 5 show offline payment modes + Ops coupon pool; on step 6 show the pending hall-booking badge; hit **Publish**. (90 s)
4. **Switch to Operations Admin** → *Hall Booking Approvals*: open #HB-2214, show the conflict check and the hall's process version/current step, **Approve**. Scroll to the **hall registry**: show hall leads, click **"Publish process update (demo)"** to watch the propagation toast (pending bookings re-sequenced, admins notified), then show the **Add Hall** form with its 4-step process builder. (60 s)
5. Ops → *All Events*: show the "Created by" column, assign a co-admin to Nritya Mahotsav. (30 s)
6. Ops → tour *Judges*, *Payment Links & Coupons*, *Sponsors*, *Reports & P&L* (note the loss-making event flag). (60 s)
7. **Switch back to Event Admin** → *My Events*: the shared event badge just created; → *Participants*: edit a profile and mark an offline payment received; → *Notifications*: send the ad-hoc broadcast. (45 s)

---

## 9. Open questions for leadership

- Should **ownership transfer** of an event require Super Admin, or is Ops sufficient?
- Coupon liability: when Ops creates a global coupon, whose event budget absorbs the discount in P&L?
- Should Event Admins see **their own event's P&L**, or only revenue (expenses stay Ops-only)?
- Retention period and access rules for the audit log.
- Do "Results only" / "Broadcast only" shared-access levels suffice, or do we need a custom per-event permission builder?

---

*End of Design Document v2.0 — IIG StageX Admin Console. All 18 referenced screens are live in `stagex-admin-wireframes.html` and demo-ready.*
