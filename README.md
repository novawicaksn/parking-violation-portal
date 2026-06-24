# Parking Violation Portal

Working slice for the Tan Digital assignment. The repository includes:

- `backend` in Go (API gateway and modular services in code boundaries)
- `frontend` in React + TypeScript
- PostgreSQL as storage

## What this slice covers

1. Officer submits a violation with plate, violation type, location, timestamp, photo.
2. System calculates fine from currently active rule version.
3. Officer can publish a new rule version.
4. Member pays a fine using mocked payment scenarios (`success` or `failed`).
5. Transaction history shows each violation with the fine and the applied rule version snapshot.

## Architecture at a glance

- Single frontend talks only to API gateway (`backend/cmd/api/main.go`).
- Gateway coordinates module-like service boundaries inside backend:
  - Rule service (rule versions, active rules)
  - Violation service (submission, fine calculation)
  - Payment service (mock charge + account deduction)
  - Notification service (invoice notification record)
- PostgreSQL stores source-of-truth and immutable rule snapshots per violation.

## Prerequisites

- Go `1.22+`
- Node.js `20+`
- PostgreSQL `14+`

## Run locally

### 1) Start PostgreSQL

You can use local PostgreSQL or Docker. Example with Docker:

```bash
docker run --name parking-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=parking_violation -p 5432:5432 -d postgres:16
```

If you already have PostgreSQL installed locally, make sure it is running and that the `parking_violation` database exists.

### 2) Run backend

From `backend`:

```bash
go mod tidy
go run ./cmd/api
```

If the server fails with a connection error to `localhost:5432`, PostgreSQL is not running yet.
If you already see `api gateway listening on :8080`, do not start a second backend instance on the same port. Stop the other process first, or set `API_ADDR=:8081` and use `VITE_API_URL=http://localhost:8081` on the frontend.
If your terminal is already inside `D:\TKJ\parking-violation-portal\backend`, do not run `cd backend` again.

Optional environment variables:

- `DATABASE_URL` (default: `postgres://postgres:postgres@localhost:5432/parking_violation?sslmode=disable`)
- `API_ADDR` (default: `:8080`)
- `FRONTEND_ORIGIN` (default: `http://localhost:5173`)
- `SEED_OFFICER_ID` (default seeded UUID)
- `SEED_MEMBER_ID` (default seeded UUID)
- `SEED_MEMBER_PLATE` (default `B1234CD`)
- `SEED_MEMBER_BALANCE` (default `1500000`)

### 3) Run frontend

From `frontend`:

```bash
npm install
npm run dev
```

If you are currently inside `backend`, move back to the repository root first, then enter `frontend`:

```powershell
Set-Location d:\TKJ\parking-violation-portal
Set-Location .\frontend
```

If you are already at the repository root, go straight to `frontend` and skip the first command.

Optional env:

- `VITE_API_URL` (default: `http://localhost:8080`)

Open `http://localhost:5173`.

### If the default ports are already in use

This workspace has been verified with:

```powershell
# terminal 1
Set-Location d:\TKJ\parking-violation-portal\backend
$env:API_ADDR=':8081'
go run ./cmd/api

# terminal 2
Set-Location d:\TKJ\parking-violation-portal\frontend
$env:VITE_API_URL='http://localhost:8081'
npm run dev -- --port 5175 --strictPort
```

Open `http://localhost:5175` in that case.

## Demo flow

1. Choose `Officer One` account.
2. Submit a violation (upload any image, pick time, type, location).
3. Publish a new rule version from the rules panel.
4. Switch to `Member One` account.
5. In unpaid list, choose `success` or `failed`, then charge.
6. Check transaction history table for applied rule version and status.

## Notes and trade-offs

- This slice keeps services modular inside one Go process to stay focused on the assignment scope.
- Notification is represented as an async-friendly record; queue/broker can be introduced later.
- Photo is stored as base64 text for simplicity. Object storage is a better production choice.
- Rule versioning is immutable for issued violations using `rule_snapshot` and `rule_version_number`.
- The required draw.io diagrams are included in [design/flow.drawio](design/flow.drawio), [design/flow.svg](design/flow.svg), [design/erd.drawio](design/erd.drawio), and [design/erd.svg](design/erd.svg).

## Security simplifications for assignment

- Authentication is mocked by selecting a user in UI and passing `X-User-ID`.
- No production auth/session flow implemented.
