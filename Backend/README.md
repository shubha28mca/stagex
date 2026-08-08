# IIG StageX — Backend (Go + Postgres)

Participant-facing API for the StageX platform. Implements the flows from
`ClientDesignWeb.md` / `ClientDesignMobile.md` against a shared account model
(**one phone = one family, many people**). Master data (event types, taxonomy,
coupons, halls, judges, sponsors) is owned by the Admin Console and lives in
`admin_`-prefixed tables — this service reads it but never writes it.

## 1. Architecture

Every domain module follows the same five-file layered shape so the codebase is
predictable and its parts are reusable:

```
model.go       data shapes + request/response DTOs
repository.go  persistence contract (interface) + Postgres implementation
service.go     business rules (unit-tested with fake repositories)
controller.go  HTTP adapter (decode → call service → encode)
routes.go      wiring onto the shared router
```

```
cmd/server/main.go            explicit wiring: middleware → routes → controllers → services → repos
internal/
  config/                     env-driven configuration (one place)
  platform/                   cross-cutting infrastructure (no domain logic)
    auth/                     JWT issue/verify, bcrypt, protect middleware
    crypto/                   AES-GCM (Aadhaar at rest), Verhoeff, masking
    database/                 pgx pool + embedded migration runner
    httpx/                    uniform JSON envelope + typed errors
    logger/                   slog JSON logger
    middleware/               CORS, request logging, panic recovery
    database/migrations/      *.sql — creation, modification & seed data
  auth/ people/ events/ coupons/ registrations/ payments/
  catalog/ myevents/ certificates/ feedback/    domain modules
docs/openapi.yaml             the service ⇄ frontend contract
```

Request flow: `middleware → route → controller → service → repository → Postgres`.

## 2. Data model

- **Admin master tables** (migration `001_admin_masters.sql`), all `admin_`-prefixed:
  `admin_event_types`, `admin_age_bands`, `admin_categories` (self-referencing
  taxonomy), `admin_coupons`, `admin_halls`, `admin_judges`, `admin_sponsors`.
- **Client tables** (migration `002_client_tables.sql`): `families`,
  `otp_challenges`, `people`, `events`, `event_categories`, `registrations`,
  `entries`, `payments`, `certificates`, `feedback`, `devices`.
- **Seeds** (`003_seed.sql`): master data + two demo events with categories.

Migrations are embedded in the binary and applied automatically on boot; each
runs in a transaction and is tracked in `schema_migrations`.

## 3. Security notes

- Aadhaar is validated (Verhoeff), **encrypted at rest** (AES-256-GCM) and only
  ever returned masked (`XXXX-XXXX-1234`).
- Passwords are bcrypt-hashed; OTPs are stored hashed, expire, and lock after 3
  attempts.
- All mutating participant endpoints require a Bearer JWT and are strictly
  family-scoped (you can never address another family's people or registrations).
- CORS allows exactly one configurable frontend origin.

## 4. Run locally

### With Docker (recommended)
From the repository root (`Q:\Stagex`):

```powershell
docker compose up --build
```

This starts Postgres, the API (migrations run automatically) and the frontend.
API: http://localhost:8080 · Frontend: http://localhost:5173

### Without Docker
Requires Go 1.23+ and a running Postgres.

```powershell
$env:DATABASE_URL = "postgres://stagex:stagex@localhost:5432/stagex?sslmode=disable"
go run ./cmd/server
```

## 5. Test

```powershell
go test ./...
```

Business logic (coupons pricing, OTP/auth, registration eligibility, payment
attempt rules, Aadhaar crypto) is covered by table-driven unit tests using
in-memory fake repositories — **no database is required to run the tests**.

## 6. API contract

The full contract is in [`docs/openapi.yaml`](docs/openapi.yaml). Import it into
Swagger UI / Postman, or view it at https://editor.swagger.io.

Response envelope: success is `{"data": ...}`; error is
`{"error": {"code": "...", "message": "..."}}`.

### Quick smoke test (dev returns the OTP in the response)

```powershell
# 1. Send an OTP for registration
$otp = (Invoke-RestMethod -Method Post http://localhost:8080/api/auth/otp/send `
  -ContentType application/json -Body '{"phone":"9876543210","purpose":"register"}').data.devOtp

# 2. Register
$auth = Invoke-RestMethod -Method Post http://localhost:8080/api/auth/register `
  -ContentType application/json `
  -Body (@{phone="9876543210";name="Priya";password="Str0ngPass!";otp=$otp} | ConvertTo-Json)
$token = $auth.data.token

# 3. Browse events (public)
Invoke-RestMethod http://localhost:8080/api/events

# 4. Add a person (protected)
Invoke-RestMethod -Method Post http://localhost:8080/api/people `
  -Headers @{Authorization="Bearer $token"} -ContentType application/json `
  -Body '{"name":"Diya","dob":"2015-06-01","gender":"female","aadhaar":"234123412346","relationship":"Daughter"}'
```

## 7. Configuration

All configuration is environment-driven (see [`.env.example`](.env.example)).
Key variables: `DATABASE_URL`, `JWT_SECRET`, `AADHAAR_KEY`, `CORS_ALLOW_ORIGIN`,
`HTTP_PORT`. Production boot fails fast if the insecure default secrets are left
unchanged.
