# FOGLINE

Decision-making trainer for degraded communication environments — SIH 2026, problem statement 26248.

Instructors run exercises in which information reaching trainees is deliberately delayed, dropped, partial or contradictory; the system records what each trainee knew when they decided, and produces an After Action Review.

> **Content rule.** This is an educational simulation. Every scenario, place, entity and objective is fictional and abstract. No real-world operational planning, targeting, weapon employment, unit deployment or sensitive information belongs in this repository.

The full design is in [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md).

## Status

Working today:

- API skeleton, authentication with three roles, web app with sign-in and role-aware routing.
- **Scenario builder.** An instructor can create a scenario, add ordered phases, schedule synthetic reports, communication degradation (delay, dropout, partial, conflicting, forced-normal) and decision points, see it all on a timeline, preview it as a run sheet, and publish it. Drafts save automatically.

- **Simulation engine and live sessions (API only).** A published scenario can be run: instructors open a lobby, trainees join by code, and the engine delivers, delays, drops, truncates and contradicts information per team while recording every event. Trainees send messages and submit decisions; instructors start, pause, resume, change speed and end. Everything is available over REST and a WebSocket stream.

Not built yet: the screens for running an exercise (lobby, trainee workspace, instructor monitor) and the After Action Review. The engine is driven through the API and its tests for now.

## Layout

```
apps/web/        Next.js 16 (App Router), TypeScript, Tailwind 4, shadcn/ui
services/api/    Go, Gin, GORM, PostgreSQL
  cmd/server       HTTP server
  cmd/migrate      migration CLI
  migrations/      SQL migrations, embedded in the binary
  internal/
    config/          environment loading and validation
    domain/          entities (no framework imports)
    auth/            password hashing, JWT
    engine/          the simulation core: pure, deterministic, no I/O
    runtime/         live sessions: one engine each, persist-then-broadcast, subscriber hub
    service/         use-cases and data-dependent authorisation
    repository/      interfaces, GORM implementation, in-memory implementation for tests
    transport/httpapi/  routes, middleware, validation, error rendering
    platform/        logger, database
docs/            design
```

## Prerequisites

Go 1.26+, Node 22+, and PostgreSQL 14+ (or Docker for `make db`).

## Run it

```sh
# 1. Database
make db                      # PostgreSQL 17 in Docker on :5432, creates fogline and fogline_test
                             # or use your own server and create a database yourself

# 2. API
cd services/api
cp .env.example .env         # then set JWT_SECRET (openssl rand -hex 32)
                             # and BOOTSTRAP_ADMIN_EMAIL / BOOTSTRAP_ADMIN_PASSWORD
go run ./cmd/server          # applies migrations, creates the admin, listens on :8080

# 3. Web
cd apps/web
cp .env.example .env.local
npm install
npm run dev                  # http://localhost:3000
```

Sign in as the bootstrap admin, or register at `/register` to get a trainee account. Instructor accounts are created by an admin through `POST /api/v1/admin/users` (there is no screen for it yet).

## Configuration

Both apps validate their environment at startup and refuse to start on bad values, listing every problem at once. The variables are documented in [services/api/.env.example](services/api/.env.example) and [apps/web/.env.example](apps/web/.env.example).

Two that matter:

- `JWT_SECRET` (API) must be at least 32 characters.
- `API_URL` (web) is where the Next.js server reaches the API. It is read when `next build` runs, so set it before building.

## How authentication works

- `POST /auth/login` and `/auth/register` return a JWT (HS256) and also set it as an `HttpOnly`, `SameSite=Lax` cookie named `fogline_token`. The API accepts either `Authorization: Bearer` or the cookie.
- The browser only talks to the Next.js origin; `/api/v1/*` is proxied to the API, so the cookie is first-party.
- Every authenticated request re-reads the user, so deactivating an account or changing its role takes effect immediately rather than when the token expires.
- Self-registration always creates a `TRAINEE`. Other roles are assigned by an admin. An admin cannot demote or deactivate their own account.
- In the web app, `proxy.ts` redirects requests with no cookie to `/login`; the layouts then verify the cookie with the API and redirect users away from other roles' areas. The API is the only thing that actually protects data.

There is no refresh token: a session lasts `JWT_TTL` (default 8h) and then the user signs in again.

## API

Base path `/api/v1`. Errors always have this shape:

```json
{ "error": { "code": "VALIDATION_FAILED", "message": "…", "details": { "email": "…" }, "requestId": "…" } }
```

| Method | Path | Access | |
|---|---|---|---|
| GET | `/healthz` | public | process is up |
| GET | `/readyz` | public | process is up and the database answers |
| POST | `/api/v1/auth/register` | public | create a trainee account |
| POST | `/api/v1/auth/login` | public | |
| POST | `/api/v1/auth/logout` | public | clears the cookie |
| GET | `/api/v1/auth/me` | signed in | current user |
| GET | `/api/v1/admin/users?limit=&offset=` | ADMIN | |
| POST | `/api/v1/admin/users` | ADMIN | create a user with any role |
| PATCH | `/api/v1/admin/users/:id` | ADMIN | `displayName`, `role`, `isActive` |
| GET | `/api/v1/scenarios` | INSTRUCTOR, ADMIN | summaries; instructors see their own |
| POST | `/api/v1/scenarios` | INSTRUCTOR, ADMIN | `{title}`; creates a draft |
| GET | `/api/v1/scenarios/:id` | owner, ADMIN | full document plus outstanding issues |
| PUT | `/api/v1/scenarios/:id` | owner, ADMIN | replace a draft's content; needs the current `revision` |
| DELETE | `/api/v1/scenarios/:id` | owner, ADMIN | refused once a session has used it |
| POST | `/api/v1/scenarios/:id/publish` | owner, ADMIN | `{confirmFictional: true}`; refused while errors remain |
| POST | `/api/v1/scenarios/:id/unpublish` | owner, ADMIN | back to draft; refused once a session has used it |
| GET | `/api/v1/sessions` | signed in | instructor: sessions they run; trainee: sessions they joined |
| POST | `/api/v1/sessions` | INSTRUCTOR, ADMIN | `{scenarioId, name?, seed?}`; opens a lobby for a published scenario |
| POST | `/api/v1/sessions/join` | TRAINEE | `{joinCode}`; placed on the smallest team |
| GET | `/api/v1/sessions/:id` | instructor of it, or a participant | role-filtered |
| PATCH | `/api/v1/sessions/:id/participants/:pid` | instructor | `{team}`; lobby only |
| POST | `/api/v1/sessions/:id/start` `pause` `resume` `end` | instructor | |
| PATCH | `/api/v1/sessions/:id/speed` | instructor | `{speed}`, 0.25 to 20; 5 means 1 real second = 5 simulated |
| GET | `/api/v1/sessions/:id/state` | instructor or participant | what that caller may see right now |
| GET | `/api/v1/sessions/:id/timeline?afterSeq=&limit=` | instructor | the full record |
| POST | `/api/v1/sessions/:id/messages` | participant | `{channelId, body}` |
| POST | `/api/v1/sessions/:id/decisions` | participant | `{decisionPointId, optionId, rationale, confidence?}` |
| GET | `/api/v1/sessions/:id/ws` | instructor or participant | WebSocket stream, server to client |

## Scenarios

A scenario is saved and loaded as one document: metadata, channels, and phases, each phase holding its reports, communication rules and decision points.

- **Times are offsets from the start of the phase** that owns the item, so resizing an earlier phase moves later content with it. The scenario's estimated duration is the sum of its phase durations rather than a separately entered number.
- **The timeline is derived, not stored.** "Report arrives", "delay ends", "decision point closes" and "phase transition" entries are all computed from the phases, reports, rules and decision points. The *Add event* menu in the builder creates the underlying item.
- **Saving and checking are separate.** A draft may be incomplete; every save returns `issues` (errors block publishing, warnings do not). Only malformed documents are rejected: bad ids, values out of range, references to channels that do not exist.
- **Saves carry a revision.** A save based on a stale revision gets `409 REVISION_CONFLICT` instead of overwriting newer work, which is how the builder detects a second tab.
- **Published scenarios are locked.** Revert to draft to edit; that is refused once an exercise session has used the scenario.
- Item ids are generated in the browser so new items can reference each other before the first save. The API inserts them with plain `INSERT`s scoped to the scenario, so an id belonging to another scenario is rejected rather than taken over.

## Simulation engine

`internal/engine` is the core. It imports no HTTP, WebSocket or database code, never reads the system clock, and has no random source: every call is given the wall time, and anything "random" is a hash of the session seed. The same scenario, seed and inputs produce a byte-identical timeline, however often the engine is polled.

| Part | File | Job |
|---|---|---|
| Clock | `clock.go` | wall time to simulation time; pause, resume, speed (integer arithmetic) |
| Scheduler | `scheduler.go` | future work in a total order: time, then kind, then authored order |
| Event processor | `processor.go` | runs due work: phases, rules starting and ending |
| Information delivery | `delivery.go` | reports and trainee messages: recipients, channel, degrade, deliver / schedule / drop |
| Communication degradation | `degradation.go` | a pure function from (item, active rules, seed) to an outcome |
| Decisions | `decisions.go` | open and close decision points; record a decision with its information snapshot |
| Timeline recorder | `recorder.go` | append-only log; hands out copies |
| State and views | `state.go`, `views.go` | what changes during a run; what a trainee or instructor may see |

**Delivery states.** `NORMAL`, `DELAYED`, `DROPPED`, `PARTIAL`, `CONTRADICTORY`. When several rules apply: a forced-normal window overrides everything, then dropout, then partial (strictest wins), and delays add up. A rule's window is `[start, end)`.

**Who sees what.** Information is addressed to teams. A trainee is sent a report only when it is actually delivered to their team, with its content as delivered and its sent and received times; never its delivery state, never that it conflicts with another, never anything about a report that was dropped or is still in transit. Communication degradation itself is not announced to trainees. The same filter builds the snapshot a client gets on connect, so reloading cannot reveal more than the live stream did. Instructors receive every timeline event.

**The server is authoritative.** A decision request carries a decision point, an option, a rationale and an optional confidence, and nothing else is read from it. Time, team, response time and the information picture are produced by the engine; deadlines are enforced by the server clock at the moment of submission.

**The record.** Every event gets a sequence number, simulation time and wall time. It is written to PostgreSQL before any client is told about it, and `timeline_events` refuses `UPDATE`, `DELETE` and `TRUNCATE` at the database level. If the database is unavailable, events queue in order and nothing is broadcast until they are stored.

### Realtime stream

`GET /api/v1/sessions/:id/ws` (cookie or bearer auth; the `Origin` must be in `CORS_ALLOWED_ORIGINS`). The first frame is `session.snapshot`; every frame is `{type, simMs, payload}`. The stream is one-way: commands go through the REST endpoints.

| Trainees receive | Instructors also receive |
|---|---|
| `simulation.started` `paused` `resumed` `completed` `time_update` | `simulation.speed_changed` |
| `phase.changed` | `phase.completed` |
| `report.received` (own team) | `report.generated` `report.delayed` `report.dropped` |
| `message.received` (own channels) | `message.sent` `message.delayed` `message.dropped` |
| `decision.opened` `decision.closed` `decision.submitted` (own) | `communication.degraded` `communication.restored` |

### Limits to know about

- **A live exercise does not survive an API restart.** Engines are in memory; on startup, sessions left running are marked ended. The record up to that point is kept.
- **One API process.** There is no cross-process fan-out yet.
- Items still in transit when an exercise ends are never delivered; they appear in the record as delayed, with no delivery.

## Database

Schema changes go through SQL files in `services/api/migrations` (golang-migrate); GORM `AutoMigrate` is not used. In development the server applies pending migrations on start; in production run `go run ./cmd/migrate up` (or set `DB_AUTO_MIGRATE=true`).

`make migrate-down` rolls back everything and destroys all data.

## Checks

```sh
make check        # lint + typecheck + tests + build, for both apps
make test-api     # go vet + go test -race
make test-web     # vitest
```

### End-to-end

`apps/web/e2e` drives the scenario builder through a real browser: create, add phases, reports, degradation and a decision point, save, reload, edit, preview, publish. It needs the whole stack running and an admin account to create its instructor with:

```sh
cd apps/web
npx playwright install chromium      # once
npm run build && npm start           # with the API running too
E2E_ADMIN_EMAIL=... E2E_ADMIN_PASSWORD=... npm run test:e2e
```

Each run adds one instructor and one published scenario to the database it runs against.

### Database tests

The PostgreSQL integration tests run only when `TEST_DATABASE_URL` is set, and are skipped otherwise. They **drop every table** in that database, so point it at a database used for nothing else:

```sh
TEST_DATABASE_URL='postgres://fogline:fogline@localhost:5432/fogline_test?sslmode=disable' make test-api
```
