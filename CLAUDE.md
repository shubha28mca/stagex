# IIG StageX — Project Context

Context file for AI-assisted work on this repo. Read this first; it explains what
the project is, how the four services fit together, where things live, and how to
run and test it **on this machine** (macOS 12 Monterey, Intel, 8 GB).

---

## 1. What this is

A containerized full-stack platform for **IIG StageX**, a cultural-event
registration product (dance/music competitions). It has two completely separate
user-facing halves that share one Postgres database:

| Half | Who uses it | Services |
|---|---|---|
| **Participant app** | Families registering children for events | `Backend` (Go API) + `Frontend` (React) |
| **Admin console** | Operational Admin & Event Admin staff | `AdminBackend` (Go API) + `AdminFrontend` (React) |

Super Admin is deliberately **out of scope**.

The implementation was driven from design documents that still live in the repo:

- [files (2)/ClientDesignWeb.md](files%20(2)/ClientDesignWeb.md) — participant web design
- [files (2)/ClientDesignMobile.md](files%20(2)/ClientDesignMobile.md) — participant mobile design
- [files (3)/StageX_Admin_Design.md](files%20(3)/StageX_Admin_Design.md) — admin console design
- Matching HTML wireframes sit beside each `.md`

These are the **source of truth for requirements**. When adding a feature, check
them first.

---

## 2. The four services

Each service is built and shipped as its **own image** so it can be deployed
independently. Ports are fixed and referenced in many places — don't change one
without changing all of them (see §6).

| Service | Compose name | Port | Health endpoint |
|---|---|---|---|
| Postgres 16 | `db` | 5432 | `pg_isready` |
| Participant API (Go) | `api` | 8080 | `GET /api/health` |
| Participant web (React → nginx) | `web` | 5173 → 80 | — |
| Admin API (Go) | `admin-api` | 8081 | `GET /admin/health` |
| Admin console (React → nginx) | `admin-web` | 5174 → 80 | — |

**Startup order matters.** `admin-api` waits for `api` to be *healthy*, not just
started, because `api` runs the migrations that create the `events` and `admin_*`
master tables. `admin-api` then layers its own migrations on top. See
[docker-compose.yml](docker-compose.yml).

### Credentials & secrets (local only)

- Postgres: user / password / database all `stagex`
- `api` JWT secret: `local-dev-jwt-secret`; Aadhaar key: `local-dev-aadhaar-key`
- `admin-api` JWT secret: `local-dev-admin-jwt-secret` (deliberately different)
- Production boot **refuses** these defaults — see `Validate()` in both
  [Backend/internal/config/config.go](Backend/internal/config/config.go) and
  [AdminBackend/internal/config/config.go](AdminBackend/internal/config/config.go)

### Seeded demo admins

Created automatically by `idSvc.Bootstrap(ctx)` in
[AdminBackend/cmd/server/main.go](AdminBackend/cmd/server/main.go), defined in
[AdminBackend/internal/identity/identity.go:75](AdminBackend/internal/identity/identity.go#L75):

| Role | Email | Password |
|---|---|---|
| Operational Admin | `ops@stagex.test` | `Ops@12345` |
| Event Admin | `event@stagex.test` | `Event@12345` |

---

## 3. Architecture conventions

### Backend (both Go services)

Every domain module follows the same five-file layering. Follow it for new modules.

```
model.go       → structs / DTOs
repository.go  → Postgres access behind an interface
service.go     → business rules (unit-testable, no DB needed)
controller.go  → HTTP decode / encode
routes.go      → RegisterRoutes(mux, controller, guards...)
```

Repositories sit **behind interfaces** so services are testable without a
database, and storage is swappable. Existing tests (`*_service_test.go`) rely on
this — they need no Postgres.

`cmd/server/main.go` is the single explicit wiring file in each backend. You can
read that one file and trace a request:
`middleware → route → controller → service → repository → Postgres`.

Shared plumbing lives under `internal/platform/`:

| Package | Responsibility |
|---|---|
| `config` | env-driven config with dev defaults + prod safety `Validate()` |
| `database` | pgxpool connection (30 s connect retry) + embedded SQL migration runner |
| `auth` | JWT issue/verify; admin side adds `RequireRole` |
| `crypto` | AES-256-GCM for Aadhaar at rest (`Backend` only) |
| `httpx` | JSON response/error helpers |
| `logger` | structured `slog` |
| `middleware` | `Chain`, `Recover`, `RequestLogger`, `CORS` |

### Migrations

Plain `.sql` files, `go:embed`-ed into the binary, applied on every server start.
Tracked in a `schema_migrations` table; each file runs in its own transaction, so
a failure never half-applies. Filenames are zero-padded — **lexical order is
execution order**.

- Participant: [Backend/internal/platform/database/migrations/](Backend/internal/platform/database/migrations/)
  — `001_admin_masters`, `002_client_tables`, `003_seed`, `004_people_soft_delete`,
  `005_event_extras`, `006_hall_seat_maps`, `007_seed_hall_seat_map`,
  `008_ticket_booking`
- Admin: [AdminBackend/internal/platform/database/migrations/](AdminBackend/internal/platform/database/migrations/)
  — `001_admin_users` … `006_event_media`

**To add a migration:** drop in the next-numbered file. Never edit an applied
one — it won't re-run, so environments will silently diverge.

### Database table split

This separation is a core design decision — respect it.

- **`admin_`-prefixed tables** = Ops-owned master data. Participants get
  read-only access through `/api/catalog/*`, never direct writes.
  `admin_event_types`, `admin_age_bands`, `admin_categories`, `admin_coupons`,
  `admin_halls`, `admin_judges`, `admin_sponsors`, `admin_users`, `admin_crew`,
  `admin_vendors`, `admin_event_crew`, `admin_event_expenses`,
  `admin_event_media`, `admin_event_sponsors`, `admin_event_vendors`,
  `admin_notifications`, `admin_notification_config`
  Seat maps live here too: `admin_hall_seat_maps`, `admin_hall_seat_zones`,
  `admin_hall_seats`, `admin_hall_seat_elements`.
- **Participant/client tables** (unprefixed): `families`, `otp_challenges`,
  `people`, `events`, `event_categories`, `registrations`, `entries`, `payments`,
  `certificates`, `feedback`, `devices`, `bookings`, `booking_items`,
  `booking_payments`, `event_seat_status`
- **Views**: `v_hall_seat_pricing` (the one definition of what a seat costs) and
  `v_event_booking_stats` (admin's "how many tickets are booked").

> **Migration ownership gotcha.** `Backend`'s migrations run *before*
> `AdminBackend`'s (compose waits for `api` to be healthy). So any table BOTH
> services touch must be created in `Backend/…/migrations/`, even when it is
> `admin_`-prefixed and only the Admin API writes to it. That is why seat maps
> live in `006_hall_seat_maps.sql` on the participant side.

### Admin role gating

The two admin route trees are **disjoint and share no path** — `/admin/ops/*` vs
`/admin/event/*`, each behind its own role guard (`RoleOps` / `RoleEvent`). An
admin hitting the wrong tree is rejected by the API and redirected by the SPA.

- **Operational Admin** — full CRUD on all master data, plus platform-wide event
  oversight and unrestricted edit/delete of any event or participant.
- **Event Admin** — creates/publishes/edits/deletes **only their own** events,
  manages categories inside them, edits their participants. Ownership comes from
  `002_event_ownership.sql`.

### Frontends (both React + Vite)

Structure is identical in both:

```
src/config/index.js   ← SINGLE source of truth for the API base URL
src/api/client.js     ← the ONE fetch wrapper; reads the config, attaches JWT
src/api/endpoints.js  ← typed endpoint functions
src/components/       ← reusable library, depends only on theme.css
src/theme/theme.css   ← the StageX design system, one file
src/context/          ← AuthContext (token + user in localStorage)
src/layout/           ← app shell
src/pages/            ← screens
```

Notable pages added for these features:

| Page | Route | What it does |
|---|---|---|
| `AdminFrontend/…/ops/SeatMapDesigner.jsx` | `/ops/halls/:hallId/seat-map` | drag-and-drop seat designer |
| `AdminFrontend/…/ops/SeatMapsPage.jsx` | `/ops/seat-maps` | halls + arrangement status |
| `AdminFrontend/…/ops/TicketSalesPage.jsx` | `/ops/ticket-sales` | tickets booked, settle refunds |
| `AdminFrontend/…/ops/SeatOccupancyPage.jsx` | `/ops/events/:eventId/seats` | per-seat occupancy + reservation report |
| `Frontend/…/BookTicketsPage.jsx` | `/events/:id/book` | seat picker → pay |
| `Frontend/…/MyBookingsPage.jsx` | `/my/bookings` | history + cancellation |

**Nothing else hard-codes a URL.** `src/config/index.js` reads
`VITE_API_BASE_URL` / `VITE_ADMIN_API_BASE_URL`, falling back to
`localhost:8080` / `localhost:8081`. In Docker these arrive as **build args** in
`docker-compose.yml` — they are baked in at build time, so changing the API URL
requires a **rebuild**, not a restart.

localStorage keys: `stagex.token` / `stagex.family`, and
`stagex.admin.token` / `stagex.admin.user`.

`AdminFrontend/src/components/CrudResource.jsx` is a generic CRUD table+form —
reuse it for new master-data screens instead of writing another page by hand.

### Hall seat maps (Ops-designed, client-consumed)

A hall has **at most one** seating arrangement (`UNIQUE (hall_id)`). Ops designs
it in a drag-and-drop editor; it is then the geometry *and pricing* reference
for every event in that hall.

- **Price rule, defined once:** `COALESCE(seat.price, zone.price, 0)` — a seat
  inherits its tier unless Ops set a per-seat override. Encoded in
  `v_hall_seat_pricing`; never re-implement it.
- **Seat identity is `(seat_map_id, section, row_label, seat_number)`.** Saving
  the map upserts on that natural key, so `admin_hall_seats.id` is **stable
  across redesigns** — which is what stops a re-save orphaning sold tickets.
- `row_index` is the **distance from the stage** (0 = front row), stored
  explicitly because row letters skip I and O and balconies restart lettering.
- Publishing is gated: no sellable seat may be unpriced.
- Editor: [AdminFrontend/src/pages/ops/SeatMapDesigner.jsx](AdminFrontend/src/pages/ops/SeatMapDesigner.jsx).
  Row geometry comes from the **server** (`POST /admin/ops/seat-maps/generate-rows`)
  so the editor and stored maps cannot drift.
- Rules + tests: [seatmaps_logic.go](AdminBackend/internal/operationaladmin/seatmaps_logic.go),
  [seatmaps_logic_test.go](AdminBackend/internal/operationaladmin/seatmaps_logic_test.go).

### Ticket booking (the audience journey)

Separate from *registering a performer*. Flow:
`seat picker → hold → checkout → pay → confirm`, then history and cancellation.

**Concurrency is the whole design.** Every claim funnels through one lockable
row per `(event_id, seat_id)` in `event_seat_status`:

1. `INSERT … ON CONFLICT DO NOTHING` materializes the rows;
2. `SELECT … ORDER BY seat_id FOR UPDATE` — **the ORDER BY prevents deadlocks**
   between overlapping requests;
3. state is re-read *under the lock*, so the loser sees the winner's write and
   is told which seats went (409 naming them).

**Expiry is judged by TIME, never by a flag** (`status='held' AND
hold_expires_at > now()`), so a stalled sweeper can never wedge a seat. The
sweeper in `service.go` is housekeeping only — it is not what makes the rule
true.

**The payment-in-flight edge case:** a booking in `payment_processing` keeps its
seats for `PAYMENT_GRACE` past the deadline. If money still lands after the
seats are gone, `DecideConfirm` **cancels with an offline refund rather than
double-selling**. See the decision table in
[tickets/logic.go](Backend/internal/tickets/logic.go).

- **Refunds are offline only.** Cancelling files a `pending_offline` work item;
  Ops marks it paid at `/ops/ticket-sales`. Nothing reverses money automatically.
- **Cancellation closes at the start of the event's first day.** The server
  decides (`canCancel` / `cannotCancelWhy`); the client only renders that.
- **Bookings are never deleted.** `booking_items` snapshots section/row/price so
  history survives a seat-map redesign; cancelled items are marked, not removed.
- Config: `SEAT_HOLD_TTL` (default 15m), `PAYMENT_GRACE` (default 5m).
- Rules + tests: [tickets/logic.go](Backend/internal/tickets/logic.go),
  [tickets/logic_test.go](Backend/internal/tickets/logic_test.go).

### Seat occupancy & the seat-reservation report (Ops)

`GET /admin/ops/events/{id}/seat-occupancy` returns **every** mapped seat with a
resolved state — `available` / `booked` / `held` / `blocked` / `house` — plus,
for sold seats, the buyer, the booking reference and **what was actually paid**
(which can differ from list price after a coupon). Unlike the buyer's seat map,
this one deliberately **includes blocked and house seats**, because those are
what Ops has to account for when arranging a hall.

`GET …/seat-report?format=csv|pdf|json` generates the report on demand: occupancy
counts, seat value (list vs realised), per-tier breakdown, ticket finance,
cancellation reasons, and the event P&L. CSV lists every seat; PDF is the
printable summary and omits available seats.

**The P&L now includes ticket income** — it did not before, so the number was
wrong for any event selling reserved seats. Ticket money is stated as
`gross − refunds = net`, and the net figure feeds `totalIncome`:

```
TotalIncome = registrations + ticketIncome(net) + sponsor + vendor
NetPL       = TotalIncome − (hall + crew + expenses)
```

`refundsPending` is reported as an outstanding **liability**, never subtracted a
second time. Rules + tests: [seatreport_logic.go](AdminBackend/internal/operationaladmin/seatreport_logic.go),
[seatreport_logic_test.go](AdminBackend/internal/operationaladmin/seatreport_logic_test.go).

> Occupancy is resolved by `OccupancyStatus()`, which applies the **same
> time-based expiry + payment-grace rule** as the participant API — so Ops and
> buyers can never disagree about whether a seat is taken. A seat Ops blocked
> reports as `blocked` even if it carries stale hold state: the layout decision
> outranks the sales state.

---

## 4. API surface

Full contract: [Backend/docs/openapi.yaml](Backend/docs/openapi.yaml).

**Participant** (`/api/*` — `people`, `registrations`, `payments`, `my/*`,
`feedback` are JWT-protected):

```
POST   /api/auth/register|login|otp/send|otp/verify
GET    /api/catalog/event-types|categories|age-bands
GET    /api/events            GET /api/events/{id}
POST   /api/coupons/validate
GET    /api/people            POST /api/people
PATCH  /api/people/{id}       DELETE /api/people/{id}
POST   /api/registrations
POST   /api/payments/order    POST /api/payments/confirm
GET    /api/my/events|certificates|notifications
POST   /api/feedback
GET    /api/health

# ticket booking (all protected)
GET    /api/events/{id}/seat-map           seat picker + per-seat availability
POST   /api/tickets/hold                   claim seats (409 names lost seats)
DELETE /api/tickets/hold/{groupId}         release early
GET    /api/tickets/hold/{groupId}/quote   priced checkout (?coupon=)
POST   /api/tickets/bookings               hold -> booking
POST   /api/tickets/bookings/{id}/pay      open a payment attempt
POST   /api/tickets/bookings/{id}/confirm  settle it (idempotent)
GET    /api/my/bookings                    history (incl. cancelled)
GET    /api/my/bookings/{id}
POST   /api/my/bookings/{id}/cancel        reason required
GET    /api/tickets/cancellation-reasons
```

**Admin** (`/admin/*`):

```
POST   /admin/auth/login      GET /admin/auth/me
/admin/ops/*     seat-maps, halls/{hallId}/seat-map (GET/POST/PUT/DELETE,
                 /publish, /unpublish, /seats), seat-maps/generate-rows
                 ticket-stats, bookings, events/{id}/bookings,
                 bookings/{id}/refund
                 events/{id}/seat-occupancy   per-seat available/paid/held/blocked
                 events/{id}/seat-report      ?format=csv|pdf|json
                 event-types, age-bands, categories, coupons, halls, judges,
                 sponsors, vendors, crew  (full CRUD)
                 events, participants, dashboard, oversight,
                 events/{id}/crew|expenses|pnl|sponsors|vendors|report
/admin/event/*   events (+publish), events/{id}/categories|certificates|crew|
                 media|notifications|report, participants,
                 participants/offline, dashboard, ref/*
GET    /media/*  uploaded event media (served from MEDIA_ROOT)
GET    /admin/health
```

---

## 5. Running locally on THIS machine

### ⚠️ Docker Desktop does not work here

This Mac is **macOS 12.7.6 (Monterey), Intel, 8 GB RAM, 2 physical cores**.
`/Applications/Docker.app` is version 4.85.0 with
`LSMinimumSystemVersion = 14.0` — **it can never launch on macOS 12**. That is why
`docker` is absent from `PATH`.

**The chosen workaround is [Colima](https://github.com/abiosoft/colima)** — a
Lima-backed Linux VM that provides a real Docker daemon. The `docker` CLI and
`docker compose` then work exactly as documented, and
[docker-compose.yml](docker-compose.yml) is used **unchanged**.

Colima also needs a hypervisor. Apple's `vz` (Virtualization.framework) backend
requires **macOS 13+**, so here the VM runs under **QEMU** — `brew install qemu`
is a hard prerequisite, and Colima exits with
`cannot use vmType: 'qemu' … qemu-img not found` without it.

Do **not** "fix" the setup by reintroducing Docker Desktop on this machine.

### Commands

```bash
colima start                       # boot the Docker VM (once per reboot)
docker compose up --build -d       # or: make up
docker compose ps                  # or: make ps
docker compose logs -f api         # or: make logs
docker compose down                # stop, keep data   (make down)
docker compose down -v             # WIPE db + media    (make reset)
colima stop                        # release the VM's RAM
```

See [setup/setup-macos.sh](setup/setup-macos.sh) for the automated path and
§7 for the exact install steps that were run.

### Performance expectations on this hardware

2 cores / 8 GB with a VM in the middle is tight. First `--build` compiles two Go
binaries and runs two Vite production builds **inside the VM** — budget
**15–25 minutes**. Subsequent starts are seconds. If a build gets OOM-killed,
give the VM more memory (`colima stop && colima start --memory 5`) or build the
services one at a time (`docker compose build api`, then `web`, …).

### Build mode

This project is set up for **production-style builds**, matching what ships:
each React app is built to static assets and served by **nginx** inside its
image; each Go service is a compiled binary in a slim image. There is no Vite
HMR in this configuration — **a frontend change requires
`docker compose up --build -d web`** (or `admin-web`) to appear.

If iterating on frontend code becomes painful, the escape hatch is to run just
that one app natively — `cd Frontend && npm install && npm run dev` — pointed at
the containerized API, since `src/config/index.js` already defaults to
`localhost:8080`.

---

## 6. Gotchas

1. **Ports are wired in four places.** Changing e.g. 5173 means updating
   `docker-compose.yml` (port map **and** the `CORS_ALLOW_ORIGIN` env on `api`),
   `vite.config.js`, and `.env.example`. CORS is **single-origin** — a mismatch
   produces a silent browser failure, not a server error.
2. **Frontend API URLs are baked in at build time** via build args. Restarting is
   not enough; rebuild.
3. **`admin-api` depends on `api` being healthy.** Starting `admin-api` alone
   against an empty database will fail — its migrations assume the participant
   tables exist.
4. **Never edit an applied migration.** Add a new numbered file.
5. **Aadhaar numbers are Verhoeff-validated and AES-256-GCM encrypted at rest,
   displayed masked.** A valid test value is `234123412346`. Don't log or return
   plaintext.
6. **Dev-only conveniences are gated on `AppEnv != "production"`** — notably the
   OTP being returned in the response. Keep new dev shortcuts behind the same gate.
7. **Automation scripts are PowerShell (`.ps1`).** All smoke scripts under
   `Backend/scripts/` and `AdminBackend/scripts/` are Windows-only and **do not
   run on macOS**. Test manually via `curl`, or port them to `.sh`.
8. **`docker compose down -v` destroys the database and all uploaded media.**
   Use plain `down` to stop.
9. **Two `.zip` files and their extracted `files (2)` / `files (3)` folders are
   duplicates**, and the folder names contain spaces — always quote paths.
10. **Go toolchain is 1.26.5 but `go.mod` declares 1.23.** That's fine (forward
    compatible); don't bump `go.mod` without a reason.
11. **Never lock `event_seat_status` rows without `ORDER BY seat_id`.** Two
    overlapping multi-seat holds will deadlock instead of queueing.
12. **Never judge a hold by `status` alone** — always `AND hold_expires_at >
    now()`, plus the payment-grace check. `tickets.Availability()` is the one
    implementation; reuse it rather than writing the predicate in SQL.
13. **Confirmation must stay idempotent.** `booking_payments.provider_ref` is
    UNIQUE and `DecideConfirm` returns `AlreadyDone` for a replayed success — a
    duplicate webhook must never sell a seat twice.
14. **Seat-map saves upsert on the natural key.** Changing a seat's
    section/row/number is effectively delete + insert, so it gets a NEW id;
    don't "tidy" numbering on a map with sold tickets.
15. **`reportPaymentGrace` in the admin report must track `PAYMENT_GRACE` in
    the participant API.** They are separate constants in separate services;
    a mismatch only changes whether a mid-payment seat reads as held, but keep
    them in step.
16. **`node` is not installed on this Mac** and Homebrew would build it from
    source. To build a frontend, fetch the official prebuilt tarball to a temp
    dir and put its `bin` on `PATH` — see §5.

---

## 7. Environment as verified on this machine

| Tool | Status |
|---|---|
| macOS | 12.7.6 Monterey, x86_64, 8 GB RAM, 4 logical / 2 physical cores |
| Homebrew | 6.0.15 at `/usr/local` (Intel prefix), Monterey bottles available |
| Go | 1.26.5 at `/usr/local/bin/go`, `GOPATH=~/go` ✅ meets 1.23+ |
| Docker Desktop | installed but **unusable** (needs macOS 14+) ❌ |
| Colima + docker CLI + compose | installed as the Docker Desktop replacement |
| Node / npm | not on the host — only needed inside the build images |
| Postgres | not on the host — runs as the `db` container |
| `gh` CLI | not installed |
| git identity | **not configured** (`user.name` / `user.email` empty) |

GitHub remote is `git@github.com:shubha28mca/stagex.git`, SSH auth works as
`shubha28mca`. The Claude Code account is `sugyan.patnaik@iigtechnology.in`
(IIG Technology) — so pick deliberately which email should author commits.

---

## 8. Testing

```bash
cd Backend      && go test ./...   # natively — Go 1.26 is installed, no DB needed
cd AdminBackend && go test ./...
```

Unit tests need **no database**: coupon pricing, OTP/auth, registration
eligibility, payment attempt rules (3-attempt retry), Aadhaar crypto, the
seat-map rules (`operationaladmin`) and the booking rules (`tickets`).

**End-to-end scripts** need a running stack. The originals are PowerShell
(Windows-only); these two are `bash` and run on this Mac:

```bash
./AdminBackend/scripts/seatmap_smoke.sh    # seat-map designer + pricing API
./Backend/scripts/booking_smoke.sh         # full buyer journey
```

`booking_smoke.sh` includes a **real concurrency test**: two buyers race for one
seat in parallel and it asserts exactly one wins. Run the api with
`SEAT_HOLD_TTL=45s` to also cover hold expiry (otherwise those checks skip).

**Building a frontend on this machine** — `node` is absent and Homebrew would
compile it, so use the official prebuilt binary:

```bash
curl -LO https://nodejs.org/dist/v22.23.2/node-v22.23.2-darwin-x64.tar.gz
tar -xzf node-v22.23.2-darwin-x64.tar.gz -C /tmp
export PATH="/tmp/node-v22.23.2-darwin-x64/bin:$PATH"
cd Frontend && npm ci && npm run build
```

Manual end-to-end walkthrough:

1. http://localhost:5173 → **Register** → 10-digit number → **Send OTP** (the OTP
   is shown in dev) → name + password → **Create account**
2. **My People** → add a person with Aadhaar `234123412346`
3. **Discover** → open an event → select the person → apply coupon `EARLYBIRD20`
   or `FLAT100` → **Pay** (try *Simulate failed attempt* for the retry counter)
4. **My Events** shows the registration and entry IDs
5. http://localhost:5174 → log in as both demo admins, confirm `/ops/*` and
   `/event/*` reject each other

Seed data (`003_seed.sql`) ships demo events **Nritya Mahotsav** and
**Taal Tarang**, plus event types, taxonomy and the two coupons.

---

## 9. Working agreement

- **Untracked junk:** `.DS_Store` files are strewn about and untracked. Don't
  commit them; a root `.gitignore` would be a welcome small fix.
- Repo has exactly one commit (`0d06380 Initial commit`) on `main`. No CI, no
  branch protection, no PR template.
- **Workflow for changes:** make the change → rebuild the affected service
  locally → verify in the browser → only then commit/push.
- Match the existing style: these files carry **substantial explanatory comments
  and package-level doc comments**. New code should too — it's the established
  convention here, not decoration.
