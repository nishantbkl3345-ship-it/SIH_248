# SIH 26248 — Immersive Multi-Domain Decision-Making Trainer for Degraded Communication Environments

**Implementation plan · v1 · 2026-10-04**
Working name: **FOGLINE**. Repository state at time of writing: empty directory, no git history. Nothing to migrate or preserve.

> **Domain constraint (binding on all contributors).** This is an educational decision-making trainer. All scenarios, entities, places and objectives are fictional and abstract. No real-world operational planning, targeting, weapon employment, unit deployment, sensitive information or actionable tactics — in code, seed data, demo scripts or documentation. Scenario content is reviewed against this rule before merge (see §18.6).

---

## 0. Decisions that shape everything else

| # | Decision | Why |
|---|---|---|
| D1 | **Server-authoritative simulation.** The Go server owns the clock, scenario state and every delivery. Clients render only what they are sent. | Information isolation is the product. A trainee must not be able to find a dropped report in browser devtools. |
| D2 | **Per-recipient delivery records.** Every report and message is fanned out into one `delivery` row per recipient, holding what was *intended* and what *actually happened*. | This single table answers "information received / delayed / dropped" for the AAR and makes decision snapshots cheap. |
| D3 | **Append-only timeline as the source of truth.** Every engine effect is a `timeline_event` with a monotonically increasing `seq`. The AAR is a pure projection of it. | "The engine must record what actually happened." Also gives replay, reconnection and audit for free. |
| D4 | **Degradation engine is a pure function.** `Evaluate(intent, activeRules, seed) → outcome`. No clock, no I/O, no global RNG. | Deterministic and table-testable; the same seed reproduces the same exercise. |
| D5 | **One goroutine (actor) per exercise session.** All mutations of a session go through its command channel. | No locks in domain code, strict event ordering, trivial pause/resume. |
| D6 | **Client-agnostic realtime protocol.** A versioned JSON envelope over WebSocket; the web UI is one renderer. | The future AR/VR client (WebXR/Unity) speaks the same protocol with no backend change. |
| D7 | **Modular monolith, single binary.** Redis and horizontal scaling are deferred. | Hackathon scope; one exercise is tens of users, not thousands. Seams are kept (§5.7) so it can be split later. |

---

## 1. Phase 1 — Requirements decomposition

### 1.1 Functional requirements

**Scenario engine (FR-S)**
- FR-S1 Author scenarios composed of phases, scheduled events, information reports, channels, decision points and degradation rules.
- FR-S2 Run a scenario against a simulation clock that can start, pause, resume, end and run at 1×/2×/4× speed.
- FR-S3 Inject **delays** on a channel (fixed or range).
- FR-S4 Inject **dropouts** (channel blackout or per-item drop, deterministic or probabilistic).
- FR-S5 Deliver **missing/partial** information (field-level redaction of a report).
- FR-S6 Deliver **conflicting** reports (two sources, same topic, incompatible claims).
- FR-S7 Change **source/channel reliability** over time, and show the trainee a reliability label that may itself be stale.
- FR-S8 Fire **events during the exercise**: scheduled, conditional (triggered by a decision or state), and manual instructor injects.
- FR-S9 Record intended vs. actual outcome for every delivery.

**Multiplayer (FR-M)**
- FR-M1 Multiple trainees join one session with a join code; assigned to teams and roles.
- FR-M2 Team and cross-team channels with real-time messaging, subject to the same degradation as reports.
- FR-M3 All clients see a synchronised sim clock, phase and exercise status.
- FR-M4 Reconnect without losing or gaining information (replay of own deliveries only).
- FR-M5 Presence (connected / idle / disconnected) visible to the instructor.

**Instructor dashboard (FR-I)**
- FR-I1 Scenario CRUD, clone, validate, publish.
- FR-I2 Configure degradation rules on a timeline.
- FR-I3 Create a session from a scenario, manage lobby, assign teams.
- FR-I4 Start / pause / resume / end; change speed.
- FR-I5 Live monitor: ground truth vs. each trainee's view, all communications, all decisions, scenario state.
- FR-I6 Manual injects: push a report, toggle a degradation rule, broadcast a notice.
- FR-I7 Time-stamped instructor observations attached to the timeline.

**After Action Review (FR-A)**
- FR-A1 Generate an AAR on session end (regenerable).
- FR-A2 Individual, team and communication timelines.
- FR-A3 Information received / delayed / dropped / partial / contradictory, per trainee.
- FR-A4 Decisions with rationale and the information snapshot at decision time.
- FR-A5 Scenario events and performance metrics.
- FR-A6 Export as PDF, JSON and CSV.

**Platform (FR-P)**
- FR-P1 JWT auth, three roles (ADMIN, INSTRUCTOR, TRAINEE), route- and resource-level RBAC.
- FR-P2 Admin: user management, system configuration, audit log.

### 1.2 Non-functional requirements

| Area | Target |
|---|---|
| Determinism | Same scenario + seed + same trainee inputs at same sim-times ⇒ identical timeline. Engine has no hidden time or randomness source. |
| Latency | Server → client fan-out p95 < 200 ms on LAN; clock drift between clients < 250 ms. |
| Scale (MVP) | 1 instructor + 30 trainees per session; 10 concurrent sessions on one 2-vCPU node. |
| Isolation | A trainee socket never receives a payload not addressed to that trainee. Enforced server-side and covered by tests. |
| Durability | Timeline events persisted before fan-out. A server restart resumes a session in `PAUSED` from the last persisted `seq`. |
| Resilience | Client auto-reconnect with exponential backoff and `lastSeq` resume. |
| Security | bcrypt/argon2id password hashing, short-lived access tokens, rate-limited auth, input validation at every boundary, audit log of privileged actions. |
| Accessibility | Degradation states never conveyed by colour alone (icon + label). Keyboard-operable decision panel. |
| Portability | `docker compose up` brings up the full stack offline — venue Wi-Fi must not matter. |
| Extensibility | Realtime protocol versioned; spatial hints optional on entities for a later XR renderer. |

### 1.3 MVP vs. advanced

**MVP (must be in the demo):** auth + RBAC · scenario library with one polished seeded scenario · scenario builder (form + timeline list) · sessions with join code and teams · live engine with all five delivery states · team chat through degraded channels · decision panel with rationale and confidence · instructor live monitor with ground-truth/trainee-view split · manual injects · pause/resume/end · AAR viewer · PDF + JSON export · audit log.

**Advanced (only after MVP is green):** drag-and-drop timeline editor · conditional event triggers UI · replay scrubber · cross-session comparison · scenario import/export as JSON · speech-to-text radio channel with audio degradation · WebXR "ops room" client · LLM-drafted lessons-learned (instructor-approved, clearly labelled) · Redis-backed multi-node hub · SSO.

### 1.4 Core workflows

1. **Author** — Instructor creates scenario → phases → reports/events → channels → degradation rules → decision points → validate → publish.
2. **Stage** — Instructor creates session from a published scenario (snapshot + seed) → shares join code → trainees join → instructor assigns teams/roles → all ready.
3. **Run** — Start → briefing → clock runs → engine emits reports/events through degradation → trainees read, communicate, decide with rationale → instructor monitors, injects, annotates → end.
4. **Review** — AAR generated → instructor adds observations and lessons → review with trainees → export.
5. **Administer** — Admin manages users, config, reads audit log.

---

## 2. Phase 2 — Roles and permissions

| Capability | ADMIN | INSTRUCTOR | TRAINEE |
|---|:-:|:-:|:-:|
| Manage users, roles, system config | ✔ | | |
| View audit log | ✔ | | |
| Create / edit / publish scenarios | ✔ | ✔ (own + shared) | |
| Create session, manage lobby, assign teams | ✔ | ✔ (own) | |
| Start / pause / resume / end, speed, injects | | ✔ (own session) | |
| See ground truth and all trainee views | | ✔ (own session) | |
| Add observations, edit lessons learned | | ✔ | |
| Join session by code, mark ready | | | ✔ |
| Receive reports, send messages on member channels | | | ✔ |
| Submit decisions with rationale | | | ✔ |
| View AAR | ✔ | ✔ full | ✔ own + own team, after release |
| Export AAR | ✔ | ✔ | own summary only |

Two layers of authorisation:
- **Role** (from JWT claim) — coarse route gating via middleware.
- **Resource** — session ownership for instructors; session membership, team and channel membership for trainees. Checked in the service layer, not the handler, so WebSocket commands and REST share the same checks.

Within a team, a trainee holds a **seat** (e.g. `LEAD`, `LAND_CELL`, `MARITIME_CELL`, `AIR_CELL`, `NETWORK_CELL`). Seats are scenario-defined strings, not hard-coded; reports and decision points are addressed to seats. `LEAD` is the only seat allowed to commit a team decision.

---

## 3. Phase 3 — System architecture

```
┌────────────────────────── Browser (Next.js) ──────────────────────────┐
│  Instructor app        Trainee app        Admin app      [future XR]  │
│  REST (TanStack Query) + WS client (Zustand store, seq-ordered reducer)│
└───────────────┬───────────────────────────────┬───────────────────────┘
                │ HTTPS /api/v1                 │ WSS /ws?ticket=…
┌───────────────▼───────────────────────────────▼───────────────────────┐
│                         Go API (single binary)                         │
│  Gin router → middleware (request-id, recover, CORS, auth, RBAC, rate) │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐ ┌─────────────┐ │
│  │ auth     │ │ scenario │ │ session  │ │ aar       │ │ admin/audit │ │
│  └──────────┘ └──────────┘ └────┬─────┘ └─────▲─────┘ └─────────────┘ │
│                                 │             │                        │
│  ┌──────────────────────────────▼─────────────┴──────────────────────┐ │
│  │ Session Runtime (one actor per live session)                      │ │
│  │  Clock → Event Engine → Degradation Engine → Delivery Scheduler   │ │
│  │                     ↓ effects                                     │ │
│  │  Recorder (timeline_events, deliveries, decisions)  → Hub fan-out │ │
│  └───────────────────────────────────────────────────────────────────┘ │
│  Realtime Hub: connections, rooms (session / team / user / instructor) │
└───────────────────────────────┬────────────────────────────────────────┘
                                │ GORM
                        ┌───────▼────────┐
                        │  PostgreSQL 16 │
                        └────────────────┘
```

### 3.1 Frontend architecture
- Next.js App Router, TypeScript strict. Route groups: `(auth)`, `(instructor)`, `(trainee)`, `(admin)`; each with its own layout and a role guard in `middleware.ts` plus a server-side check.
- **Server state:** TanStack Query against REST. **Live state:** one Zustand store per session, fed by a single WebSocket; a reducer applies envelopes strictly in `seq` order and requests a resync on a gap.
- The WS client lives in `lib/realtime/` and has no React dependency, so it can be reused by a WebXR client.
- Types for REST and WS payloads are **generated from the Go structs** (`tygo`) into `packages/protocol` — one source of truth, no drift.
- shadcn/ui primitives; a small set of domain components (`ReportCard`, `ReliabilityBadge`, `ChannelStatus`, `DecisionCard`, `TimelineLane`, `SimClock`).
- Forms: react-hook-form + zod. Charts in the AAR: Recharts.

### 3.2 Backend architecture
Modular monolith with a strict dependency direction: `transport → service → domain ← repository`. The `domain` and `engine` packages import nothing from Gin, GORM or the network.

- `transport/http` — Gin handlers, DTO binding/validation.
- `transport/ws` — connection lifecycle, envelope codec, command routing.
- `service` — use-cases and authorisation checks.
- `engine` — clock, event engine, degradation engine, scheduler (pure).
- `runtime` — the per-session actor that wires engine ↔ recorder ↔ hub.
- `repository` — GORM implementations behind interfaces.
- `aar` — projections and exporters.

### 3.3 WebSocket architecture
- **Handshake:** client calls `POST /sessions/:id/ws-ticket` with its JWT → receives a single-use, 30-second ticket → opens `GET /ws?ticket=…`. Tokens never appear in URLs or logs.
- **Hub:** per-connection read and write pumps with a bounded send buffer; a slow consumer is disconnected rather than allowed to block the session.
- **Rooms:** `session:{id}`, `team:{id}`, `user:{id}`, `instructor:{sessionId}`. Fan-out targets are computed by the runtime, never by the client.
- **Envelope:**
  ```json
  { "v": 1, "type": "report.delivered", "seq": 412, "sessionId": "…",
    "simTime": 180, "wallTime": "2026-…Z", "payload": { } }
  ```
  `seq` is per recipient stream, so a trainee cannot infer hidden events from gaps.
- **Resume:** on reconnect the client sends `session.resume {lastSeq}`; the server replays that user's events after `lastSeq`, or sends a full `session.snapshot` if too far behind.
- **Heartbeat:** ping/pong every 15 s; presence derived from it.
- Client commands carry a client-generated `cmdId`; the server replies `ack`/`error` with the same id and treats repeats idempotently.

### 3.4 Simulation engine
- `Clock` holds `simTime` (integer seconds), `status` (`LOBBY|BRIEFING|RUNNING|PAUSED|ENDED`) and `speed`. A ticker drives it in production; tests call `Advance(n)` directly.
- Each tick the runtime calls `engine.Step(state, simTime) → []Effect`. Effects are data (`EmitReport`, `ActivateRule`, `ChangePhase`, `OpenDecisionPoint`, `ReleaseDelivery`, …). The runtime persists them, then fans them out.
- Pausing stops the ticker; scheduled deliveries are keyed on sim-time, so they are frozen with it.
- `clock.sync` is broadcast every 5 s and on every status change; clients interpolate locally.

### 3.5 Degradation engine — see §5.

### 3.6 Event engine
Three trigger kinds, one queue:
- **Scheduled** — fire at `simTime == at`.
- **Conditional** — fire when a predicate over session state becomes true (e.g. *decision D3 = option B*, *phase = 2*, *flag X set*). Predicates are a small JSON expression tree (`all/any/eq/gte`), not a scripting language.
- **Manual** — an instructor inject, timestamped at the current sim-time.

Event actions: emit report, activate/deactivate degradation rule, change phase, open/close decision point, set state flag, change source reliability, broadcast notice. Conditional and manual events are recorded with their trigger cause so the AAR can explain branching.

### 3.7 Decision recording — see §6.

### 3.8 AAR generation — see §7.

### 3.9 Authentication and authorisation
- Email + password, argon2id. Access JWT 15 min (HS256, claims: `sub`, `role`, `exp`, `jti`); refresh token 7 days, rotating, stored hashed, delivered as `httpOnly; Secure; SameSite=Strict` cookie.
- Middleware: `RequireAuth` → `RequireRole(...)`; service layer: `CanAccessSession`, `CanAccessChannel`, `CanCommitDecision`.
- Login and refresh rate-limited per IP and per account.
- Trainees may also be provisioned as lightweight accounts by an instructor (name + join code) to keep the lobby fast in a classroom.

### 3.10 Logging and audit
- Structured JSON logs (`slog`) with `request_id`, `user_id`, `session_id`. No message bodies or rationale text in application logs.
- `audit_logs` table for privileged actions: login, role change, scenario publish, session control, inject, AAR export. Append-only; admin read-only.
- The exercise `timeline_events` table is separate from the audit log: one is training data, the other is system accountability.

### 3.11 Error handling
- REST errors follow one shape: `{ "error": { "code": "SESSION_NOT_RUNNING", "message": "…", "details": {…}, "requestId": "…" } }`. Typed domain errors map to HTTP status in one place.
- WS command failures return an `error` envelope with the originating `cmdId`; the connection stays open.
- A panic inside a session actor is recovered, logged, and the session is moved to `PAUSED` with an instructor-visible alert — one bad scenario must not take down other sessions.
- The frontend has a route-level error boundary and a persistent connection-status indicator (real connectivity, visually distinct from *simulated* degradation so trainees never confuse the two).

---

## 4. Phase 4 — Domain model

The suggested names were kept where they fit. Changes and the reasoning:

| Suggested | Used | Reason |
|---|---|---|
| InformationReport | **ReportTemplate** (authored) + **Delivery** (runtime, per recipient) | Separates ground truth from what each person actually saw — the core of the product. |
| Message | **Message** + **Delivery** | Trainee messages go through the same degradation path as reports. |
| Trainee | **Participant** (User × Session × Team × Seat) | A user is a trainee only in the context of a session. |
| DecisionRationale | field on **Decision** + **DecisionSnapshot** | Rationale is 1:1 with a decision; the snapshot is the separate, heavy part. |
| Decision | **DecisionPoint** (authored prompt) + **Decision** (trainee's answer) | Distinguishes the question from the response; enables latency measurement. |
| — | **Source** | Reliability belongs to a source (e.g. "Relay Station K-7"), and conflicts are between sources. |
| — | **Claim** | Structured key/value facts inside a report, so partial delivery and contradiction detection are computable rather than free text. |
| — | **ScenarioVersion** | Sessions pin an immutable snapshot; editing a scenario never alters a past AAR. |

### 4.1 Authoring aggregate (immutable once published as a version)

- **Scenario** — title, summary, objectives (abstract), difficulty, tags, owner, status.
- **ScenarioVersion** — version number, full JSON snapshot, checksum. Sessions reference this.
- **ScenarioPhase** — order, name, start sim-time, briefing text.
- **TeamTemplate / SeatTemplate** — team names and seats expected by the scenario.
- **Source** — fictional origin of information; `baseReliability` (A–E), `displayedReliability`.
- **CommunicationChannel** — name, kind (`TEAM|CROSS_TEAM|BROADCAST|FEED`), member seats, base latency.
- **ReportTemplate** — source, channel, addressed seats, topic key, title, body, **claims[]**, `truthValue` (`TRUE|FALSE|UNCERTAIN`, instructor-only), `partialMask` (which claims survive a PARTIAL delivery), optional `contradicts` (id of the report it conflicts with).
- **ScenarioEvent** — trigger (scheduled/conditional/manual-template) + action list.
- **DegradationRule** — see §5.2.
- **DecisionPoint** — phase, addressed seats or team, scope (`INDIVIDUAL|TEAM`), type, prompt, opens-at, time limit, relevant topic keys.
- **DecisionOption** — label, description, authored outcome note, `score` weights (instructor-only), state flags set when chosen.

### 4.2 Runtime aggregate

- **ExerciseSession** — scenario version, instructor, join code, seed, status, speed, sim-time, started/ended.
- **Team**, **Participant** — instantiated from templates at session creation.
- **ActiveRule** — a rule currently in force, with activation cause.
- **Message** — sender participant, channel, body, sim-time.
- **Delivery** — one per (item, recipient): intended vs. actual outcome (§5.4).
- **Decision** — see §6.
- **DecisionSnapshot** — ids of deliveries and messages visible to the decider, plus state.
- **TimelineEvent** — append-only log.
- **InstructorObservation** — note, sim-time, optional target participant/team/decision, tag.
- **PerformanceMetric** — computed values per participant/team/session.
- **AARReport** — generated document (JSON), status, release flag, export records.

### 4.3 XR-readiness hooks (cost: a few nullable fields)
- `Source`, `Team` and `ReportTemplate` carry an optional `spatial` JSON (`{x,y,z,label}` on an abstract grid) — unused by the web UI, available to a 3D renderer.
- `ReportTemplate.media` optional (audio/image URL) for a future voice-radio channel.

---

## 5. Phase 5 — Degradation engine

### 5.1 Contract

```
Evaluate(intent DeliveryIntent, rules []ActiveRule, seed uint64) DeliveryOutcome
```

- `DeliveryIntent` = { itemKind (REPORT|MESSAGE), itemId, channelId, sourceId, recipientId, intendedAt (sim-s), claims[] }
- `DeliveryOutcome` = { state, deliverAt (sim-s, absent if dropped), visibleClaims[], substitutedContent?, appliedRuleIds[], reason }

Pure, total, no clock, no I/O. Everything else (scheduling the delayed release, persisting, fan-out) is the runtime's job.

### 5.2 Rule model

```
DegradationRule {
  id, name
  scope:    { channelIds[], sourceIds[], seatIds[], teamIds[], reportIds[], itemKinds[] }   // empty = any
  window:   { fromSim, toSim }            // or activated/deactivated by events / instructor
  effect:   DELAY | DROP | PARTIAL | CONTRADICT | RELIABILITY_SHIFT
  params:   DELAY      { minSec, maxSec }
            DROP       { probability }                    // 1.0 = blackout
            PARTIAL    { keepRatio | useAuthoredMask }
            CONTRADICT { variantReportId | flipClaimKeys[] }
            RELIABILITY_SHIFT { sourceId, actual, displayed }
  priority: int
}
```

### 5.3 Resolution algorithm (fixed, documented, tested)

1. Select rules whose scope matches the intent and whose window contains `intendedAt`.
2. Order by `priority` desc, then rule id — a total order, so no map-iteration nondeterminism.
3. Apply in a fixed pipeline:
   1. **DROP** — if any matching drop rule fires → state `DROPPED`, stop.
   2. **CONTRADICT** — replace content with the authored variant / flipped claims → state `CONTRADICTORY`.
   3. **PARTIAL** — remove claims per mask/ratio → state `PARTIAL` (unless already `CONTRADICTORY`; then both are recorded in `appliedRuleIds`, primary state stays `CONTRADICTORY`).
   4. **DELAY** — sum of matching delays added to channel base latency → if > 0 and state still `NORMAL`, state `DELAYED`; `deliverAt = intendedAt + delay`.
4. No matching rule → `NORMAL`, `deliverAt = intendedAt + channel.baseLatency`.

`RELIABILITY_SHIFT` does not alter a delivery; it changes source state, which changes the truthfulness of later reports selected by events and the reliability label trainees see.

### 5.4 Determinism

Probabilistic choices never draw from a shared RNG stream (stream order would depend on goroutine timing). Each draw is a keyed hash:

```
u = hash64(seed, ruleId, itemId, recipientId, purpose) / 2^64      // purpose ∈ {drop, delay, mask}
```

Consequences: results are independent of evaluation order; replaying a session reproduces it exactly; a test can assert a specific outcome for a specific seed.

### 5.5 What is recorded

`deliveries` row per (item, recipient): `intended_at`, `intended_content`, `state`, `deliver_at`, `delivered_at`, `delivered_content`, `applied_rule_ids`, `reason`, `read_at`.
Plus timeline events: `DELIVERY_SCHEDULED`, `DELIVERY_RELEASED`, `DELIVERY_DROPPED`, `RULE_ACTIVATED`, `RULE_DEACTIVATED`, `RELIABILITY_CHANGED`.

A dropped item is never sent to the recipient's socket. The instructor sees it in the ground-truth lane with a "dropped" marker.

### 5.6 Contradictory information — two mechanisms
- **Authored conflict (preferred):** two `ReportTemplate`s on the same `topicKey` from different sources with incompatible claims, linked by `contradicts`. Both are delivered normally and tagged `CONTRADICTORY` in the record (not on the trainee's screen — spotting it is the exercise).
- **Injected conflict:** a `CONTRADICT` rule swaps one recipient's copy for a variant, so two team-mates hold different versions of the "same" report. This is what makes team communication matter.

### 5.7 Golden test (the example from the brief)

| Sim-time | Setup | Expected record |
|---|---|---|
| T=120 | Report A emitted, no active rules | `NORMAL`, `deliverAt=120` |
| T=150 | Rule R1 activated: DELAY 30 s on channel C1 | `RULE_ACTIVATED` at 150; next item on C1 at T=160 → `DELAYED`, `deliverAt=190` |
| T=180 | Report B emitted; rule R2: DROP p=1.0 on report B | `DROPPED`, no `deliverAt`, nothing on recipient socket |
| T=220 | Reports C1/C2 on topic `route.status`, sources S1/S2, claims `open` vs `blocked` | two deliveries, both flagged `CONTRADICTORY`, linked |

This table is `engine/degradation/golden_test.go` and is the first test written.

---

## 6. Phase 6 — Decision recording

A decision is submitted over the WebSocket (`decision.submit`) so it is sequenced on the same timeline as everything else. The **server** builds the snapshot; the client never tells the server what it knew.

| Required field | Stored as |
|---|---|
| trainee | `participant_id` |
| team | `team_id` |
| timestamp | `wall_time` (server clock) |
| simulation timestamp | `sim_time` |
| decision type | `decision_point.type` (copied) |
| selected option | `option_id` |
| rationale | `rationale` (required, min length) + `confidence` 1–5 + optional `info_gaps` ("what I wish I knew") |
| information available at that moment | `snapshot.delivery_ids[]` — deliveries to this participant with `delivered_at ≤ sim_time`, each with state and read/unread |
| communications available at that moment | `snapshot.message_delivery_ids[]` — same query, messages |
| relevant scenario state | `snapshot.state` — phase, active flags, channel status *as visible to the trainee*, plus an instructor-only `ground_truth` block: active rules, items pending/dropped for this trainee on the decision's topics |
| response time | `response_ms` = submit − decision point opened (sim), and `since_last_relevant_info_ms` |

Additional recorded facts: `cited_delivery_ids[]` (trainee may tick the reports they relied on), `revision_of` (decisions are never edited; a change is a new row linked to the old), `is_final`.

**Team decisions.** Any member may `decision.propose`; members `decision.concur` / `decision.dissent` with a note; the `LEAD` seat `decision.commit`s. All four are recorded, so the AAR shows how consensus formed and who was working from what information.

Snapshot assembly is one indexed query on `deliveries (participant_id, delivered_at)`; it is stored as id arrays, not copied content, since deliveries are immutable.

---

## 7. Phase 7 — After Action Review

Generation is a set of pure projections over `timeline_events`, `deliveries`, `messages`, `decisions`, `observations` → one `AARDocument` JSON, stored in `aar_reports.content`. The viewer and all exporters render from that document; they never re-query raw tables.

| # | Section | Content | Source |
|---|---|---|---|
| 1 | Executive summary | Outcome in 5–6 lines: duration, participants, headline metrics, three most significant moments (auto-selected, instructor-editable) | metrics + key events |
| 2 | Scenario overview | Fictional setting, objectives, phases, teams and seats, channels, sources | scenario version |
| 3 | Timeline | Unified swim-lane view: scenario events · degradation · per-team comms · decisions | timeline_events |
| 4 | Information availability | Per participant: matrix of report × state (received / delayed +Ns / partial / dropped / contradictory), read time | deliveries |
| 5 | Communication degradation | Rule activations as bands over time, per channel; count and share of affected items | active rules + deliveries |
| 6 | Individual decisions | Per trainee, per decision: option, rationale, confidence, latency, **what they had vs. what existed** | decisions + snapshots |
| 7 | Team decisions | Proposal → concur/dissent → commit trail; information asymmetry within the team at commit time | decisions |
| 8 | Decision latency | Distribution per decision point; latency vs. information completeness | decisions |
| 9 | Contradictory-information handling | For each conflict: who received which version, time to notice (first message referencing it / verification request), action taken, decision made before or after resolution | deliveries + messages + decisions |
| 10 | Communication patterns | Volume per channel over time, who-talks-to-whom matrix, time for a report to be relayed to the team, behaviour during blackout | messages |
| 11 | Key events | Instructor-pinned and auto-detected moments | timeline + observations |
| 12 | Instructor observations | Time-stamped notes with tags | observations |
| 13 | Lessons learned | Instructor-authored; system offers prompts derived from the metrics (plain rules, e.g. "3 of 5 decisions in phase 2 were made with < 50 % of relevant information") | instructor |
| 14 | Performance metrics | Table below | performance_metrics |

### 7.1 Metrics (all computable, none subjective)

| Metric | Definition |
|---|---|
| Decision latency | sim-seconds from decision point open to final submit |
| Information completeness at decision | relevant claims held ÷ relevant claims emitted by then |
| Decision quality | authored option score, reported alongside completeness — a poor outcome with 30 % of the information reads differently from one with 100 % |
| Confidence calibration | stated confidence vs. option score |
| Contradiction detection time | conflict delivered → first acknowledgement |
| Verification behaviour | share of conflicts where the trainee asked for confirmation before deciding |
| Information sharing latency | report received → relayed on team channel |
| Unread-at-decision | relevant delivered items not opened before deciding |
| Rationale quality proxy | length, cited reports, stated gaps (flag only — never auto-graded) |
| Team alignment | share of members concurring at commit; information asymmetry index |
| Comms discipline under blackout | messages attempted on a dropped channel vs. use of alternates |

### 7.2 Export
- **PDF** — a print-styled route `/instructor/aar/[id]/print` rendered by headless Chromium (`chromedp`) on the server, so the PDF matches the viewer exactly. Fallback: browser print.
- **JSON** — the full `AARDocument` (machine-readable, future LMS integration).
- **CSV** — zipped: `decisions.csv`, `deliveries.csv`, `messages.csv`, `timeline.csv`, `metrics.csv`.
- Each export is logged to `audit_logs` and `aar_exports`.

---

## 8. Phase 8 — Screens

### 8.1 Instructor

| Screen | Route | Contents |
|---|---|---|
| Login | `/login` | Shared with all roles; redirects by role. |
| Dashboard | `/instructor` | Live sessions, recent AARs, scenario count, quick "new session". |
| Scenario Library | `/instructor/scenarios` | Cards with tags, difficulty, duration; clone, archive. |
| Scenario Builder | `/instructor/scenarios/[id]/edit` | Tabs: Overview · Phases · Teams & Seats · Sources & Channels · Reports · Decision Points · Events. Validation panel. |
| Scenario Configuration | `/instructor/scenarios/[id]/degradation` | Timeline with one lane per channel; rules as bands; preview "what seat X will see" computed by the real engine with a seed. |
| Exercise Lobby | `/instructor/sessions/[id]/lobby` | Join code (large, QR), connected trainees, drag to team/seat, ready states, seed, Start. |
| Live Exercise Monitor | `/instructor/sessions/[id]/live` | Header: clock, phase, controls. Left: teams and presence. Centre: **ground truth vs. trainee view** split. Right: decisions feed, comms feed. Bottom: inject bar (send report, toggle rule, notice) and observation box. |
| Timeline | `/instructor/sessions/[id]/timeline` | Full swim-lane timeline, filterable; live during the run. |
| Trainee Detail | `/instructor/sessions/[id]/trainees/[pid]` | Read-only mirror of that trainee's screen plus what was withheld from them. |
| AAR Viewer | `/instructor/aar/[id]` | 14 sections, sticky nav, edit observations/lessons, release to trainees. |
| AAR Export | `/instructor/aar/[id]/export` | Choose sections and format; export history. |

### 8.2 Trainee

| Screen | Route | Contents |
|---|---|---|
| Login | `/login` or `/join` | Account login, or join code + name. |
| Exercise Lobby | `/trainee/sessions/[id]/lobby` | Team, seat, team-mates, ready toggle. |
| Briefing | `/trainee/sessions/[id]/briefing` | Fictional situation, objectives, seat responsibilities, channels and sources with stated reliability. Acknowledge to continue. |
| Live Exercise | `/trainee/sessions/[id]/live` | Three-pane workspace; the next four screens are panes on desktop and tabs on mobile. |
| Incoming Reports | pane / `…/live?tab=reports` | Chronological feed; each card shows source, displayed reliability, received time, *sent* time (so a delay is discoverable), redaction markers for partial content, unread state. |
| Team Communications | pane / `…/live?tab=comms` | Channel list with status (nominal / degraded / no link), messages, "delivery uncertain" indicator on send when the channel is degraded. |
| Decision Panel | pane / `…/live?tab=decide` | Open decision points with countdown, options, mandatory rationale, confidence slider, "cite reports", "information I am missing". Team decisions show propose/concur/commit. |
| Decision History | `…/live?tab=history` | Own and team decisions with rationale; revise where allowed. |
| Exercise Completion | `/trainee/sessions/[id]/complete` | Summary of own decisions, short self-reflection form, link to AAR once released. |

Admin: `/admin/users`, `/admin/config`, `/admin/audit`.

**UI rule:** the trainee UI never states "this report was degraded". It shows only what a real recipient could observe (timestamps, missing fields, silence). The instructor UI states everything.

---

## 9. Database schema (PostgreSQL 16)

All tables: `id uuid pk default gen_random_uuid()`, `created_at`, `updated_at`. Soft delete only on `users` and `scenarios`. Money is not involved; times in sim-seconds are `integer`.

```
-- identity
users(id, email UNIQUE, password_hash, display_name, role ENUM(ADMIN,INSTRUCTOR,TRAINEE), is_active, last_login_at, deleted_at)
refresh_tokens(id, user_id→users, token_hash UNIQUE, expires_at, revoked_at, user_agent, ip)
audit_logs(id, actor_id→users NULL, action, entity_type, entity_id, metadata JSONB, ip, created_at)       -- append-only
system_config(key PK, value JSONB, updated_by→users)

-- authoring
scenarios(id, owner_id→users, title, summary, objectives JSONB, difficulty, tags TEXT[], est_duration_sec,
          status ENUM(DRAFT,PUBLISHED,ARCHIVED), current_version_id, deleted_at)
scenario_versions(id, scenario_id→scenarios, version INT, snapshot JSONB, checksum, published_by, published_at,
                  UNIQUE(scenario_id, version))
scenario_phases(id, scenario_id, ord INT, name, starts_at_sim INT, briefing TEXT)
team_templates(id, scenario_id, name, color)
seat_templates(id, team_template_id, key, title, responsibilities TEXT, can_commit BOOL)
sources(id, scenario_id, key, name, description, base_reliability CHAR(1), displayed_reliability CHAR(1), spatial JSONB)
channels(id, scenario_id, key, name, kind ENUM(TEAM,CROSS_TEAM,BROADCAST,FEED), base_latency_sec INT, member_seat_keys TEXT[])
report_templates(id, scenario_id, phase_id, source_id, channel_id, topic_key, title, body TEXT, claims JSONB,
                 truth ENUM(TRUE,FALSE,UNCERTAIN), partial_mask JSONB, contradicts_id→report_templates NULL,
                 addressed_seat_keys TEXT[], media JSONB, spatial JSONB)
scenario_events(id, scenario_id, phase_id, name, trigger_kind ENUM(SCHEDULED,CONDITIONAL,MANUAL),
                at_sim INT NULL, condition JSONB NULL, actions JSONB, ord INT)
degradation_rules(id, scenario_id, name, effect ENUM(DELAY,DROP,PARTIAL,CONTRADICT,RELIABILITY_SHIFT),
                  scope JSONB, from_sim INT NULL, to_sim INT NULL, params JSONB, priority INT, enabled BOOL)
decision_points(id, scenario_id, phase_id, key, scope ENUM(INDIVIDUAL,TEAM), type, prompt TEXT,
                opens_at_sim INT, time_limit_sec INT NULL, addressed_seat_keys TEXT[], topic_keys TEXT[], allow_revision BOOL)
decision_options(id, decision_point_id, ord, label, description, outcome_note TEXT, score JSONB, sets_flags JSONB)

-- runtime
exercise_sessions(id, scenario_version_id→scenario_versions, instructor_id→users, name, join_code UNIQUE, seed BIGINT,
                  status ENUM(LOBBY,BRIEFING,RUNNING,PAUSED,ENDED), speed NUMERIC, sim_time INT, last_seq BIGINT,
                  state JSONB, started_at, ended_at)
teams(id, session_id, template_key, name, color)
participants(id, session_id, user_id→users, team_id→teams NULL, seat_key, is_ready, connection_state, joined_at,
             UNIQUE(session_id, user_id))
active_rules(id, session_id, rule_key, source ENUM(SCENARIO,EVENT,INSTRUCTOR), activated_at_sim, deactivated_at_sim NULL,
             params JSONB, activated_by→users NULL)
messages(id, session_id, channel_key, sender_participant_id, body TEXT, sim_time INT, wall_time, client_cmd_id UNIQUE)
deliveries(id, session_id, item_kind ENUM(REPORT,MESSAGE), item_ref, recipient_participant_id, channel_key, source_key NULL,
           topic_key NULL, intended_at_sim INT, intended_content JSONB,
           state ENUM(NORMAL,DELAYED,DROPPED,PARTIAL,CONTRADICTORY), secondary_states TEXT[],
           deliver_at_sim INT NULL, delivered_at_sim INT NULL, delivered_wall TIMESTAMPTZ NULL, delivered_content JSONB NULL,
           applied_rule_keys TEXT[], reason TEXT, conflict_group UUID NULL, read_at_sim INT NULL)
decisions(id, session_id, decision_point_key, participant_id, team_id, scope, type, option_key,
          rationale TEXT, confidence SMALLINT, info_gaps TEXT, cited_delivery_ids UUID[],
          stage ENUM(PROPOSED,CONCUR,DISSENT,COMMITTED,INDIVIDUAL), revision_of→decisions NULL, is_final BOOL,
          wall_time, sim_time INT, opened_at_sim INT, response_ms BIGINT, since_last_info_ms BIGINT)
decision_snapshots(decision_id PK→decisions, delivery_ids UUID[], message_delivery_ids UUID[], unread_delivery_ids UUID[],
                   visible_state JSONB, ground_truth JSONB)
timeline_events(id, session_id, seq BIGINT, sim_time INT, wall_time, type, actor_participant_id NULL, actor_user_id NULL,
                team_id NULL, visibility ENUM(INSTRUCTOR,TEAM,PARTICIPANT,ALL), ref_type, ref_id, payload JSONB,
                UNIQUE(session_id, seq))                                                           -- append-only
instructor_observations(id, session_id, author_id, sim_time, target_type, target_id, tag, note TEXT, pinned BOOL)

-- review
performance_metrics(id, session_id, subject_type ENUM(PARTICIPANT,TEAM,SESSION), subject_id, key, value NUMERIC, unit, detail JSONB)
aar_reports(id, session_id UNIQUE, status ENUM(GENERATING,READY,FAILED), content JSONB, lessons_learned TEXT,
            released_to_trainees BOOL, generated_at, generated_by)
aar_exports(id, aar_report_id, format ENUM(PDF,JSON,CSV), sections TEXT[], file_path, exported_by, created_at)
```

Indexes that matter: `deliveries(recipient_participant_id, delivered_at_sim)`, `deliveries(session_id, deliver_at_sim) WHERE delivered_at_sim IS NULL AND state <> 'DROPPED'`, `timeline_events(session_id, seq)`, `decisions(session_id, participant_id, sim_time)`, `messages(session_id, channel_key, sim_time)`.

Runtime tables reference scenario content by **key** into the pinned `scenario_versions.snapshot`, not by FK to authoring tables, so authoring edits can never corrupt a past session. Migrations with `golang-migrate` (SQL files); GORM `AutoMigrate` is not used outside tests.

---

## 10. REST API (`/api/v1`)

**Auth**
```
POST   /auth/login                 POST /auth/refresh            POST /auth/logout
GET    /auth/me                    POST /auth/join               (join code + display name → trainee token)
```
**Admin** (ADMIN)
```
GET/POST /admin/users              GET/PATCH/DELETE /admin/users/:id
GET/PUT  /admin/config             GET /admin/audit-logs?actor=&action=&from=&to=
```
**Scenarios** (INSTRUCTOR, ADMIN)
```
GET/POST /scenarios                GET/PATCH/DELETE /scenarios/:id
POST /scenarios/:id/clone          POST /scenarios/:id/validate       POST /scenarios/:id/publish
GET  /scenarios/:id/versions       POST /scenarios/:id/preview        (engine dry-run: seat, seed → predicted deliveries)
CRUD /scenarios/:id/phases         CRUD /scenarios/:id/teams          CRUD /scenarios/:id/sources
CRUD /scenarios/:id/channels       CRUD /scenarios/:id/reports        CRUD /scenarios/:id/events
CRUD /scenarios/:id/degradation-rules                                 CRUD /scenarios/:id/decision-points (+ nested options)
```
**Sessions**
```
GET/POST /sessions                 GET /sessions/:id                  DELETE /sessions/:id (LOBBY only)
GET  /sessions/:id/participants    PATCH /sessions/:id/participants/:pid   (team, seat)
POST /sessions/:id/ready           POST /sessions/:id/ws-ticket
POST /sessions/:id/start | pause | resume | end                       PATCH /sessions/:id/speed
POST /sessions/:id/injects         (report | rule-toggle | notice)
GET  /sessions/:id/state           (role-filtered snapshot)           GET /sessions/:id/timeline?afterSeq=&types=
GET  /sessions/:id/participants/:pid/view      (instructor: that trainee's deliveries + withheld items)
GET  /sessions/:id/decisions       GET /sessions/:id/messages         GET /sessions/:id/deliveries
GET/POST /sessions/:id/observations            PATCH/DELETE /observations/:id
GET  /me/sessions                  (trainee: sessions joined)
```
**AAR**
```
POST /sessions/:id/aar/generate    GET /sessions/:id/aar              GET /aar/:id
PATCH /aar/:id                     (lessons learned, summary edits, pinned events)
POST /aar/:id/release              POST /aar/:id/exports {format, sections}
GET  /aar/:id/exports              GET /aar/exports/:exportId/download
```
**Ops:** `GET /healthz`, `GET /readyz`.

Session control is exposed over REST *and* WS; both call the same service method.

---

## 11. WebSocket events (protocol v1)

**Client → server**

| Type | Role | Payload |
|---|---|---|
| `session.resume` | any | `lastSeq` |
| `participant.ready` | trainee | `ready` |
| `message.send` | trainee | `cmdId, channelKey, body` |
| `delivery.read` | trainee | `deliveryId` |
| `decision.submit` | trainee | `cmdId, decisionPointKey, optionKey, rationale, confidence, infoGaps, citedDeliveryIds` |
| `decision.propose` / `.concur` / `.dissent` / `.commit` | trainee | as above + `proposalId`, note |
| `verification.request` | trainee | `deliveryId, note` (asks the source to confirm; scenario may answer) |
| `control.start` / `.pause` / `.resume` / `.end` / `.speed` | instructor | — / `speed` |
| `inject.report` / `inject.rule` / `inject.notice` | instructor | template key or ad-hoc body; rule key + on/off |
| `observation.add` | instructor | `note, tag, targetType, targetId` |
| `monitor.focus` | instructor | `participantId` (subscribe to a trainee mirror) |

**Server → client**

| Type | Audience | Meaning |
|---|---|---|
| `ack` / `error` | sender | Result of a command, by `cmdId` |
| `session.snapshot` | connecting client | Full role-filtered state |
| `session.status` | all | LOBBY/BRIEFING/RUNNING/PAUSED/ENDED |
| `clock.sync` | all | `simTime, speed, status, serverWall` |
| `phase.changed` | all | new phase |
| `lobby.updated` / `presence.changed` | all / instructor | membership, readiness, connection |
| `report.delivered` | addressed trainee | the delivered (possibly partial/variant) content |
| `message.delivered` | channel member | a message that survived degradation |
| `message.status` | sender | `SENT` only — the sender is never told whether it arrived |
| `channel.status` | channel members | observable status: nominal / degraded / no link |
| `decision.opened` / `decision.closed` | addressed | decision point lifecycle |
| `decision.recorded` | decider, team | confirmation; team sees team-scope decisions |
| `decision.proposal.updated` | team | propose/concur/dissent/commit trail |
| `notice.broadcast` | all | instructor notice |
| `gt.delivery` | instructor | every delivery outcome incl. dropped/pending, with rule ids |
| `gt.rule.changed` | instructor | rule activated/deactivated and cause |
| `gt.event.fired` | instructor | scenario event with trigger cause |
| `gt.decision` | instructor | every decision with snapshot summary |
| `gt.message` | instructor | every message with per-recipient outcomes |
| `aar.ready` | instructor / trainees on release | AAR available |

`gt.*` (ground truth) types are only ever written to the `instructor:{sessionId}` room. A test asserts no `gt.*` envelope can be serialised to a trainee connection.

---

## 12. Frontend routes

```
/                              → redirect by role
/login   /join
/instructor
/instructor/scenarios
/instructor/scenarios/new
/instructor/scenarios/[id]
/instructor/scenarios/[id]/edit
/instructor/scenarios/[id]/degradation
/instructor/sessions
/instructor/sessions/new
/instructor/sessions/[id]/lobby
/instructor/sessions/[id]/live
/instructor/sessions/[id]/timeline
/instructor/sessions/[id]/trainees/[pid]
/instructor/aar
/instructor/aar/[id]
/instructor/aar/[id]/export
/instructor/aar/[id]/print          (print layout, used by PDF renderer)
/trainee
/trainee/sessions/[id]/lobby
/trainee/sessions/[id]/briefing
/trainee/sessions/[id]/live         (?tab=reports|comms|decide|history)
/trainee/sessions/[id]/complete
/trainee/sessions/[id]/aar
/admin/users   /admin/config   /admin/audit
```

---

## 13. Folder structure

```
SIH_248/
├─ README.md
├─ Makefile                      # dev, test, lint, seed, demo
├─ docker-compose.yml            # postgres, api, web
├─ docker-compose.demo.yml       # production builds + seeded demo data, offline-capable
├─ .env.example
├─ docs/
│  ├─ IMPLEMENTATION_PLAN.md     # this file
│  ├─ protocol.md                # WS envelope + event reference (generated)
│  ├─ scenario-authoring.md      # incl. content rules (fictional only)
│  └─ demo-script.md
├─ apps/
│  └─ web/
│     ├─ app/
│     │  ├─ (auth)/login  (auth)/join
│     │  ├─ (instructor)/instructor/…
│     │  ├─ (trainee)/trainee/…
│     │  └─ (admin)/admin/…
│     ├─ components/
│     │  ├─ ui/                  # shadcn
│     │  ├─ exercise/            # ReportCard, ChannelStatus, SimClock, DecisionCard
│     │  ├─ monitor/             # GroundTruthSplit, InjectBar, PresenceList
│     │  ├─ builder/             # scenario forms, DegradationTimeline
│     │  ├─ timeline/            # swim-lane timeline
│     │  └─ aar/                 # one component per AAR section
│     ├─ lib/
│     │  ├─ api/                 # typed REST client
│     │  ├─ realtime/            # ws client, reducer, resume logic (framework-free)
│     │  ├─ stores/              # zustand
│     │  └─ auth/
│     ├─ middleware.ts
│     └─ e2e/                    # Playwright
├─ packages/
│  └─ protocol/                  # generated TS types (REST DTOs + WS events)
├─ services/
│  └─ api/
│     ├─ cmd/
│     │  ├─ server/main.go
│     │  └─ seed/main.go
│     ├─ internal/
│     │  ├─ config/
│     │  ├─ domain/              # entities, value objects, errors — no framework imports
│     │  ├─ engine/
│     │  │  ├─ clock/
│     │  │  ├─ events/
│     │  │  ├─ degradation/      # Evaluate + golden tests
│     │  │  └─ scheduler/
│     │  ├─ runtime/             # session actor, manager, recorder, recovery
│     │  ├─ service/             # auth, scenario, session, decision, aar, admin
│     │  ├─ repository/          # interfaces + gorm/
│     │  ├─ transport/
│     │  │  ├─ http/             # router, handlers, dto, middleware
│     │  │  └─ ws/               # hub, conn, codec, commands
│     │  ├─ aar/                 # projections/, metrics/, export/{pdf,json,csv}
│     │  ├─ audit/
│     │  └─ platform/            # db, logger, jwt, hash, ids
│     ├─ migrations/             # *.up.sql / *.down.sql
│     └─ testdata/               # scenario fixtures, golden timelines
├─ scenarios/
│  └─ tidewatch.json             # the seeded demo scenario
└─ .github/workflows/ci.yml
```

---

## 14. Implementation phases

Sized for a team of 4–6 (suggested split: 2 backend, 2 frontend, 1 scenario/AAR/demo, 1 floating/QA). Each phase ends in something runnable.

| Phase | Deliverable | Exit criteria |
|---|---|---|
| **0 Foundations** | Monorepo, compose, CI, migrations, config, logger, health, Next.js shell with shadcn, type generation | `make dev` runs all three services; CI green on lint + empty tests |
| **1 Identity** | Users, login/refresh/logout, RBAC middleware, role-guarded layouts, audit log, admin user screen | Three seeded users log in and land on the right home; forbidden routes return 403 |
| **2 Engine core (headless)** | `domain`, `clock`, `events`, `degradation`, `scheduler`; scenario JSON loader; `tidewatch.json` | Golden test (§5.7) and determinism test pass; a CLI runs a scenario and prints the timeline — **no UI yet** |
| **3 Sessions + realtime** | Session actor, recorder, hub, ws-ticket, lobby, clock sync, resume | Two browsers join by code; start/pause/resume stays in sync; kill and reconnect a tab without loss or leak |
| **4 Trainee experience** | Briefing, reports feed, comms through degradation, decision panel, snapshots, history | A full exercise is playable end-to-end by trainees; every decision has a server-built snapshot |
| **5 Instructor live** | Monitor with ground-truth split, trainee mirror, injects, observations, live timeline | Instructor can see a dropped report that a trainee cannot; manual blackout takes effect within one tick |
| **6 AAR** | Projections, metrics, 14-section viewer, PDF/JSON/CSV export, release to trainees | AAR for the demo run is complete and the PDF matches the viewer |
| **7 Authoring UI** | Scenario library, builder forms, degradation timeline, validate, publish, preview | A new short scenario can be authored and run without touching JSON |
| **8 Hardening + demo** | Seed/reset scripts, demo compose, e2e, load check, rehearsal, backup recording | Three clean rehearsals from cold start inside 8 minutes |

Order rationale: the engine is built and proven before any screen depends on it (phase 2), and authoring UI comes late because the seeded JSON scenario unblocks every other phase. **If time runs out, phase 7 degrades gracefully** to "library + configuration screen + JSON import"; phases 2–6 are non-negotiable.

Critical path: 0 → 2 → 3 → 4 → 5 → 6. Phase 1 runs in parallel with 2; phase 7 in parallel with 5–6.

---

## 15. Testing strategy

| Level | Scope | Tooling |
|---|---|---|
| Unit — engine | Table tests for every rule effect and every precedence combination; the golden example; keyed-hash distribution sanity | `go test` |
| Property | Same seed ⇒ identical outcomes regardless of evaluation order; `deliverAt ≥ intendedAt`; a dropped item has no `deliverAt`; partial content ⊆ intended content | `testing/quick` or `rapid` |
| Golden timeline | Run `tidewatch.json` with seed 42 and a scripted set of trainee inputs; diff the full timeline against `testdata/tidewatch.seed42.golden.json` | `go test -update` to regenerate deliberately |
| Clock | Pause/resume/speed with a fake ticker; no delivery released while paused | `go test` |
| Service + repository | Against real Postgres | `testcontainers-go` |
| HTTP | Handler tests incl. full RBAC matrix (role × route → expected status) generated from one table | `httptest` |
| WebSocket | In-process server with several fake clients: fan-out targets, resume, slow-consumer disconnect | `gorilla/websocket` or `coder/websocket` test clients |
| **Isolation (security-critical)** | Trainee A never receives B's deliveries, any `gt.*` event, or any dropped item — asserted on raw socket frames, and on every REST endpoint with A's token | dedicated suite, must pass in CI |
| AAR | Projection unit tests from fixture timelines; metric definitions checked against hand-computed values | `go test` |
| Frontend unit | Realtime reducer (ordering, gap → resync), decision form validation | Vitest + Testing Library |
| End-to-end | One instructor + two trainee browser contexts play the demo script; asserts the AAR contents | Playwright |
| Load | 10 sessions × 30 clients, 4× speed; watch fan-out latency and memory | k6 (ws) |
| Race | `go test -race` on the whole backend in CI | — |

CI gates: `golangci-lint`, `go vet`, `go test -race`, `tsc --noEmit`, `eslint`, Vitest, isolation suite, and the e2e demo flow on the main branch.

---

## 16. Deployment strategy

- **Local dev:** `docker compose up` → Postgres, API with hot reload (`air`), Next.js dev server. `make seed` loads users and the demo scenario.
- **Demo (primary):** `docker-compose.demo.yml` with production builds on one laptop, served on the LAN. Trainee devices join over a hotspot/router the team brings. No internet dependency; fonts and assets are bundled. `make demo-reset` restores a clean state in seconds.
- **Hosted (secondary, for remote judging):** one small VM with Docker Compose behind Caddy (automatic TLS, WebSocket pass-through), or web on Vercel + API on Fly.io/Render + managed Postgres. API and web on the same parent domain so the refresh cookie works.
- **Config:** 12-factor env vars; secrets never committed; `.env.example` documents each.
- **Migrations:** run as a one-shot container before the API starts.
- **Observability:** JSON logs to stdout, `/healthz` and `/readyz`, optional Prometheus `/metrics` (active sessions, connections, fan-out latency).
- **Recovery:** on boot the session manager finds sessions in `RUNNING`, rebuilds state from the snapshot + timeline, and resumes them as `PAUSED` with a notice to the instructor.
- **Scale-out path (not MVP):** sticky sessions by `sessionId`, Redis pub/sub between hub instances, session actor ownership via a lease. The `runtime.Manager` and `ws.Hub` interfaces are already the seams for this.

---

## 17. Phase 9 — SIH demonstration

### 17.1 Demo scenario: *Exercise Tidewatch* (entirely fictional)

A storm has disrupted the island state of **Veridia**. A joint coordination cell must decide how to route a relief convoy from **Port Halden** to the town of **Mirel**, choosing between the coastal road, the inland pass, or a sea transfer. No adversary, no force, no real geography — the problem is purely *which option to pick, when, on what information*.

- **Teams:** Team Amber, Team Cobalt (shown side by side to prove multi-team).
- **Seats (multi-domain cells):** Lead · Land Cell · Maritime Cell · Air Cell · Network Cell.
- **Sources:** Relay Station K-7 (reliable, later degraded), Harbour Watch (reliable), Survey Drone Kestrel (fast, partial), Community Net (unverified).
- **Channels:** Team Net, Coordination Net (cross-team), Field Feed.

Scripted timeline (mirrors the brief's example; run at 2× so it fits):

| Sim T | What happens | Requirement shown |
|---|---|---|
| 0 | Briefing; decision point D1 opens at T=60 | scenario engine, events |
| 120 | Report A (coastal road status) arrives immediately | NORMAL |
| 150 | Field Feed delayed 30 s | DELAYED |
| 180 | Report B (bridge condition) is dropped for Team Amber's Land Cell only | DROPPED, missing information |
| 200 | Kestrel report arrives with fields redacted | PARTIAL |
| 220 | K-7 says the inland pass is *open*; Community Net says *blocked* | CONTRADICTORY |
| 240 | K-7 reliability silently falls from A to D (label on trainee screens stays A) | changing reliability |
| 260 | Team decision D2: commit a route | team coordination, decision + rationale |
| live | Instructor injects a Team Net blackout for 20 s | event during exercise, instructor control |

### 17.2 Run-of-show (8 minutes)

Setup: projector shows the instructor monitor; two laptops/phones are trainees on different teams; ideally a judge takes a third trainee seat.

| Time | Action | Say |
|---|---|---|
| 0:00–0:45 | Problem in one sentence. Show the scenario configuration timeline with degradation bands. | "Training assumes perfect information. We make the information imperfect on purpose, and record exactly how." |
| 0:45–1:30 | Create session → join code/QR → trainees (and a judge) join → assign teams → Start. | Multiplayer, lobby, synchronised clock. |
| 1:30–2:30 | T=120–180. On the monitor split view: Report A reaches everyone; the delayed item shows a countdown on the instructor side; Report B is marked dropped for one trainee whose screen shows nothing. | "The trainee doesn't know what they don't know. The instructor does." |
| 2:30–3:30 | T=200–240. Partial report with redactions; the two conflicting reports land; trainees discuss on Team Net. | Conflicting reports, real-time team comms. |
| 3:30–4:15 | Instructor injects a comms blackout mid-discussion; a trainee's message shows "sent" but never arrives. Pause and resume once to show control. | Events during an exercise; start/pause/resume. |
| 4:15–5:15 | Trainees submit D2 with rationale and confidence; Lead commits. Monitor shows decisions arriving with "had 3 of 5 relevant reports". Instructor adds an observation. | Decision capture with information snapshot. |
| 5:15–5:30 | End exercise → AAR generates. | — |
| 5:30–7:15 | AAR: timeline lanes → information availability matrix → one decision opened to show *what they knew vs. what existed* → contradiction handling → latency chart → metrics. Export PDF, open it. | "Every claim in this report traces to a recorded event." |
| 7:15–8:00 | Architecture slide: deterministic engine (re-run with the same seed → identical timeline), server-side isolation, protocol-first design ready for an AR/VR client. | Technical depth, roadmap. |

### 17.3 Requirement → evidence map (put this on a slide)

| Problem-statement item | Where the judge sees it |
|---|---|
| Delays, dropouts, missing info, conflicting reports, changing reliability, in-exercise events | T=150, 180, 200, 220, 240, and the live inject |
| Multiple trainees, team coordination, real-time comms, synchronised state | Lobby with two teams, Team Net chat, shared clock, pause affecting all screens |
| Instructor creates/configures, controls, monitors | Configuration timeline, control bar, ground-truth split view |
| Exportable AAR with all listed elements | AAR viewer sections 3–14, PDF/JSON export |

### 17.4 Judging strategy
- **Lead with the split view.** One screen showing what was sent next to what a trainee saw is the whole idea in a single image.
- **Put a judge in a trainee seat.** Being the person whose report was dropped is more persuasive than any slide.
- **Prove determinism live** if asked: re-run with the same seed from the CLI and diff the timelines.
- **Have answers ready** for: scaling (actor per session + Redis seam), security (server-side isolation, tests on raw frames), XR (same protocol, spatial hints already in the model), content safety (fictional-only authoring rule and review), how metrics avoid subjectivity (all derived from recorded events; quality is always shown next to information completeness).
- **De-risk:** offline compose stack, own router, `make demo-reset`, a pre-generated AAR from a rehearsal run, and a 3-minute screen recording as a last resort.
- **Do not** demo the scenario builder at length; show the configuration timeline for 30 seconds and move to the live run.

---

## 18. Risks and working agreements

| # | Risk | Mitigation |
|---|---|---|
| 18.1 | Information leaks to trainee clients | Per-recipient fan-out computed server-side; isolation test suite in CI; trainee DTOs are separate types from instructor DTOs (no shared struct with `omitempty`). |
| 18.2 | Non-determinism creeps in (map order, wall-clock, goroutine timing) | Keyed-hash randomness; engine has no `time.Now()`; golden timeline test fails loudly. |
| 18.3 | Scenario builder consumes the schedule | JSON scenario first; UI last; scope fallback defined in §14. |
| 18.4 | Real vs. simulated degradation confused during the demo | Distinct, always-visible real-connection indicator; own network. |
| 18.5 | AAR metrics read as grading people | Metrics are descriptive; decision quality is always paired with information completeness; lessons learned are instructor-authored. |
| 18.6 | Scenario content drifts toward real-world material | Written authoring rules in `docs/scenario-authoring.md`; reviewer checklist on every scenario PR: fictional places and names, abstract objectives, no real organisations, equipment or tactics. |
| 18.7 | PDF rendering fails at the venue | Browser-print fallback and a pre-exported PDF. |

**Working agreements:** trunk-based with short-lived branches and PR review; the WS protocol and DB schema change only via PRs that update `packages/protocol` and migrations together; every engine change comes with a table test; no scenario content merges without the §18.6 checklist.

---

## 19. First week checklist for the executing developer

1. `git init`, add this plan, set up the monorepo skeleton from §13 and CI.
2. Write migrations for identity tables; implement login and RBAC.
3. Write `engine/degradation` with the §5.7 golden test **first**, then the clock and event engine.
4. Author `scenarios/tidewatch.json` against the domain model; build the headless CLI runner.
5. Freeze protocol v1 (§11) and generate TS types — after this, backend and frontend work in parallel.
