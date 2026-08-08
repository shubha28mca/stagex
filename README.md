# IIG StageX — Platform (Full Stack)

A clean, containerized implementation of the IIG StageX **participant** experience
and **admin console**, built from the design documents in `files (2)/` and
`files (3)/`:

- **Participant app** — Go + Postgres backend and React frontend: the family
  account, discovery, registration, payment, people, certificates.
- **Admin console** — a separate Go backend (own port) and React frontend for
  **Operational Admin** and **Event Admin** (Super Admin is out of scope). It
  writes to the **same database** but is deployed and secured independently.
- **Admin master data** — event types, taxonomy, coupons, halls, judges and
  sponsors live in `admin_`-prefixed tables (sourced from `StageX_Admin_Design.md`),
  cleanly separated from the participant/client tables.

Every frontend/backend is **built and hosted as a separate image** so they can be
deployed independently.

## Repository layout

```
Stagex/
├─ Backend/                 Go API (see Backend/README.md)
│  ├─ cmd/server/           entrypoint & wiring
│  ├─ internal/
│  │  ├─ config/            env-driven configuration
│  │  ├─ platform/          auth, crypto, database, httpx, logger, middleware
│  │  │  └─ database/migrations/   creation, modification & seed SQL
│  │  ├─ auth people events coupons registrations payments
│  │  └─ catalog myevents certificates feedback
│  ├─ docs/openapi.yaml     API contract (service ⇄ frontend)
│  └─ Dockerfile
├─ Frontend/                React app (see Frontend/README.md)
│  ├─ src/config/           SINGLE source of truth for the backend URL
│  ├─ src/api/              HTTP client + typed endpoints
│  ├─ src/components/       reusable component library
│  ├─ src/theme/            StageX design system (one CSS file)
│  ├─ src/pages/            Auth, Discover, EventDetail, MyPeople, MyEvents, Certificates
│  └─ Dockerfile
├─ AdminBackend/            Admin Go API (port 8081, SAME database)
│  ├─ cmd/server/           entrypoint & role-gated wiring
│  ├─ internal/
│  │  ├─ platform/          auth (role JWT), database, httpx, logger, middleware
│  │  ├─ identity/          admin login + mock-admin bootstrap
│  │  ├─ operationaladmin/  Ops: master CRUD + oversight + participants
│  │  └─ eventadmin/        Event Admin: events, categories, participants
│  └─ Dockerfile
├─ AdminFrontend/           Admin React console (port 5174)
│  ├─ src/config/           SINGLE source of truth for the admin API URL
│  ├─ src/components/       reusable UI + generic CrudResource
│  ├─ src/pages/ops/        Operational Admin pages
│  ├─ src/pages/event/      Event Admin pages
│  └─ Dockerfile
├─ docker-compose.yml       db + api + web + admin-api + admin-web
├─ setup/                   one-command setup scripts (windows / macos / linux)
├─ Makefile                 shortcuts: make up | down | logs | ps | reset
└─ README.md                (this file)
```

## Set up on a new machine

**The only software you need to install is Docker.** Everything else (Go, Node,
Postgres) runs inside containers — you never install them on the host.

| OS | Requirement | How the setup script installs it |
|---|---|---|
| Windows 10/11 | Docker Desktop | `winget` (uses WSL2; a one-time restart may be required) |
| macOS (Intel / Apple Silicon) | Docker Desktop | Homebrew (`brew install --cask docker`) |
| Linux | Docker Engine + Compose plugin | official `get.docker.com` script |

### Option A — one command (recommended)

Copy this folder to the new machine, open a terminal in it, and run the script
for your OS. Each script installs Docker if it's missing, waits for the engine,
then builds and starts the whole stack.

**Windows** (PowerShell):
```powershell
powershell -ExecutionPolicy Bypass -File setup\setup-windows.ps1
```

**macOS**:
```bash
chmod +x setup/setup-macos.sh && ./setup/setup-macos.sh
```

**Linux**:
```bash
chmod +x setup/setup-linux.sh && ./setup/setup-linux.sh
```

### Option B — manual (Docker already installed)

```bash
docker compose up --build -d      # or:  make up
```

The first run downloads base images and builds — give it a few minutes. After
that, starting again takes seconds.

### Everyday commands

| Task | Command | Make |
|---|---|---|
| Start / rebuild | `docker compose up --build -d` | `make up` |
| Stop (keep data) | `docker compose down` | `make down` |
| Follow logs | `docker compose logs -f` | `make logs` |
| Status | `docker compose ps` | `make ps` |
| Wipe ALL data (DB + media) | `docker compose down -v` | `make reset` |

## Access & ports

- Frontend: http://localhost:5173
- Backend API: http://localhost:8080  (health: `/api/health`)
- Admin console: http://localhost:5174
- Admin API: http://localhost:8081  (health: `/admin/health`)
- Postgres: localhost:5432 (user/pass/db all `stagex`)

Migrations and seed data run automatically on backend start, so the app has demo
events (Nritya Mahotsav, Taal Tarang), event types, taxonomy and coupons
(`EARLYBIRD20`, `FLAT100`) immediately.

### Admin console — demo accounts

Open http://localhost:5174 and log in (mock admins are seeded automatically):

| Role | Email | Password |
|---|---|---|
| Operational Admin | `ops@stagex.test` | `Ops@12345` |
| Event Admin | `event@stagex.test` | `Event@12345` |

- **Operational Admin** owns the master data: full CRUD on event types, age
  bands, taxonomy, coupons, halls, judges and sponsors, plus platform-wide event
  oversight and unrestricted edit/delete of any event or participant.
- **Event Admin** creates, publishes, edits and deletes **their own** events,
  manages the categories inside them, and edits participants of their events.
- The two areas are role-gated and **share no path** (`/ops/*` vs `/event/*`);
  an admin of the wrong role is redirected away. Both write to the same database
  as the participant app.

### Try it
1. Open the frontend, click **Register**, enter a 10-digit number → **Send OTP**
   (the dev OTP is shown), set a name + password → **Create account**.
2. **My People** → add a person (Aadhaar is validated, stored encrypted, shown
   masked). Use a Verhoeff-valid number, e.g. `234123412346`.
3. **Discover** → open an event → select the person → apply `EARLYBIRD20` →
   **Pay** (try *Simulate failed attempt* to see the 3-attempt retry counter).
4. **My Events** shows the registration and entry IDs.

## Run without Docker

- **Participant backend:** needs Go 1.23+ and a Postgres. See [`Backend/README.md`](Backend/README.md).
- **Participant frontend:** needs Node 18+. See [`Frontend/README.md`](Frontend/README.md).
- **Admin backend:** `cd AdminBackend; go run ./cmd/server` (port 8081, same `DATABASE_URL`).
- **Admin frontend:** `cd AdminFrontend; npm install; npm run dev` (port 5174).

## Test

```powershell
cd Backend; go test ./...
```

Coupon pricing, OTP/auth, registration eligibility, payment attempt rules and
Aadhaar crypto are covered by unit tests that need **no database**. The admin
API ships smoke scripts under `AdminBackend/scripts/` that exercise login, CRUD
and role separation against a running stack.

## Design decisions

- **Layered modules** — every backend domain follows `model → repository →
  service → controller → routes`, with repositories behind interfaces so services
  are unit-testable and storage is swappable.
- **Admin vs client separation** — Ops-owned master data is `admin_`-prefixed and
  read-only to participants (via `/api/catalog/*`).
- **Security** — bcrypt passwords, hashed+expiring OTPs with lockout, JWT-guarded
  and strictly family-scoped endpoints, AES-256-GCM Aadhaar at rest, single-origin
  CORS. Production boot refuses insecure default secrets.
- **Reusability** — the frontend component library depends only on the theme CSS,
  and the backend location is configured in a single file.

## Configuration

- Backend: [`Backend/.env.example`](Backend/.env.example)
- Frontend: [`Frontend/.env.example`](Frontend/.env.example) (or the
  `VITE_API_BASE_URL` build arg in `docker-compose.yml`)
