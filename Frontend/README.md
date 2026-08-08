# IIG StageX — Frontend (React + Vite)

Participant web app implementing the StageX design system and the flows from
`ClientDesignWeb.md`. Built to be **simple and reusable**: a small component
library (`src/components`) plus thin pages, with a single place to point at the
backend.

## 1. Point it at a backend — one place

The backend URL lives in exactly one file: [`src/config/index.js`](src/config/index.js).
It reads `VITE_API_BASE_URL` (from `.env` in dev, or the Docker build arg) and
falls back to `http://localhost:8080`. Every network call goes through
[`src/api/client.js`](src/api/client.js), so **no component ever hard-codes a
URL**. To re-target the API when hosting, change that one value.

## 2. Structure

```
src/
  config/index.js       SINGLE source of truth for the API base URL + storage keys
  api/
    client.js           fetch wrapper: base URL, auth header, {data}/{error} envelope
    endpoints.js        typed API surface (1:1 with the backend OpenAPI contract)
  theme/theme.css       the StageX design system — colors, type, primitives (re-theme here)
  components/           reusable library: Button, Field, Panel, Chip, Badge,
                        Stepper, StarRating, EventCard, Spinner/Alert/Empty
  context/AuthContext   session state (family + JWT), persisted to localStorage
  layout/Layout         sticky top bar + tabs
  pages/                Auth, Discover, EventDetail (3-step register+pay),
                        MyPeople, MyEvents, Certificates
  App.jsx               routes (public: Discover/EventDetail; protected: My*)
  main.jsx              entry point
```

The component library depends only on `theme.css`, so it can be lifted into
another project by copying `src/components` + `src/theme` and re-skinning the CSS
variables.

## 3. Run locally

### With Docker (recommended)
From the repository root: `docker compose up --build` — serves the app at
http://localhost:5173.

### Without Docker
Requires Node 18+.

```powershell
npm install
copy .env.example .env   # adjust VITE_API_BASE_URL if needed
npm run dev
```

Open http://localhost:5173. Make sure the backend is running (see `../Backend`).

## 4. Flows implemented

- **Auth** — phone + OTP registration and dual (password / OTP) login. In dev the
  backend returns the OTP, shown as a hint for easy testing.
- **Discover** — search + fee/rounds filters, minimal-info event cards.
- **Event detail** — 3-step registration: choose participants (per-person age
  eligibility) → payment (coupon apply + 3-attempt retry) → confirmation with
  entry IDs.
- **My People** — list + add a person (mandatory identity fields, Aadhaar masked).
- **My Events** — registrations with per-participant entry codes.
- **Certificates** — family certificate list with download links.

## 5. Build

```powershell
npm run build   # outputs to dist/
```
