-- Initial schema. Enumerations are text + CHECK so they can be extended with
-- an ordinary migration.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name  text NOT NULL,
    role          text NOT NULL CHECK (role IN ('ADMIN', 'INSTRUCTOR', 'TRAINEE')),
    is_active     boolean NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------- authoring

CREATE TABLE scenarios (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id         uuid NOT NULL REFERENCES users (id),
    title            text NOT NULL,
    summary          text NOT NULL DEFAULT '',
    objectives       jsonb,
    difficulty       text NOT NULL DEFAULT '',
    status           text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
    est_duration_sec integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_scenarios_owner ON scenarios (owner_id);

CREATE TABLE scenario_phases (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id   uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    ord           integer NOT NULL,
    name          text NOT NULL,
    starts_at_sim integer NOT NULL DEFAULT 0,
    briefing      text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scenario_id, ord)
);

CREATE TABLE communication_channels (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id      uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    key              text NOT NULL,
    name             text NOT NULL,
    kind             text NOT NULL CHECK (kind IN ('TEAM', 'CROSS_TEAM', 'BROADCAST', 'FEED')),
    base_latency_sec integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scenario_id, key)
);

CREATE TABLE information_reports (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id        uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    phase_id           uuid REFERENCES scenario_phases (id) ON DELETE SET NULL,
    channel_id         uuid REFERENCES communication_channels (id) ON DELETE SET NULL,
    source_name        text NOT NULL,
    source_reliability text NOT NULL DEFAULT 'C' CHECK (source_reliability IN ('A', 'B', 'C', 'D', 'E')),
    topic_key          text NOT NULL DEFAULT '',
    title              text NOT NULL,
    body               text NOT NULL DEFAULT '',
    claims             jsonb,
    truth              text NOT NULL DEFAULT 'TRUE' CHECK (truth IN ('TRUE', 'FALSE', 'UNCERTAIN')),
    release_at_sim     integer,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_information_reports_scenario ON information_reports (scenario_id);

CREATE TABLE scenario_events (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id  uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    phase_id     uuid REFERENCES scenario_phases (id) ON DELETE SET NULL,
    name         text NOT NULL,
    trigger_kind text NOT NULL CHECK (trigger_kind IN ('SCHEDULED', 'CONDITIONAL', 'MANUAL')),
    at_sim       integer,
    condition    jsonb,
    actions      jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_scenario_events_scenario ON scenario_events (scenario_id, at_sim);

CREATE TABLE degradation_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    channel_id  uuid REFERENCES communication_channels (id) ON DELETE CASCADE,
    name        text NOT NULL,
    effect      text NOT NULL CHECK (effect IN ('DELAY', 'DROP', 'PARTIAL', 'CONTRADICT', 'RELIABILITY_SHIFT')),
    from_sim    integer,
    to_sim      integer,
    params      jsonb,
    priority    integer NOT NULL DEFAULT 0,
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (from_sim IS NULL OR to_sim IS NULL OR to_sim >= from_sim)
);
CREATE INDEX idx_degradation_rules_scenario ON degradation_rules (scenario_id);

CREATE TABLE decision_points (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id    uuid NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    phase_id       uuid REFERENCES scenario_phases (id) ON DELETE SET NULL,
    scope          text NOT NULL DEFAULT 'INDIVIDUAL' CHECK (scope IN ('INDIVIDUAL', 'TEAM')),
    type           text NOT NULL DEFAULT '',
    prompt         text NOT NULL,
    opens_at_sim   integer NOT NULL DEFAULT 0,
    time_limit_sec integer,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_decision_points_scenario ON decision_points (scenario_id);

CREATE TABLE decision_options (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    decision_point_id uuid NOT NULL REFERENCES decision_points (id) ON DELETE CASCADE,
    ord               integer NOT NULL,
    label             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (decision_point_id, ord)
);

-- ------------------------------------------------------------------ runtime

CREATE TABLE exercise_sessions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scenario_id   uuid NOT NULL REFERENCES scenarios (id),
    instructor_id uuid NOT NULL REFERENCES users (id),
    name          text NOT NULL,
    join_code     text NOT NULL UNIQUE,
    seed          bigint NOT NULL DEFAULT 0,
    status        text NOT NULL DEFAULT 'LOBBY' CHECK (status IN ('LOBBY', 'BRIEFING', 'RUNNING', 'PAUSED', 'ENDED')),
    speed         double precision NOT NULL DEFAULT 1 CHECK (speed > 0),
    sim_time      integer NOT NULL DEFAULT 0,
    started_at    timestamptz,
    ended_at      timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_exercise_sessions_instructor ON exercise_sessions (instructor_id);

CREATE TABLE teams (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    name       text NOT NULL,
    color      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, name)
);

CREATE TABLE session_participants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id),
    team_id    uuid REFERENCES teams (id) ON DELETE SET NULL,
    seat       text NOT NULL DEFAULT '',
    is_ready   boolean NOT NULL DEFAULT false,
    joined_at  timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, user_id)
);
CREATE INDEX idx_session_participants_user ON session_participants (user_id);

CREATE TABLE messages (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id            uuid NOT NULL REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    channel_id            uuid NOT NULL REFERENCES communication_channels (id),
    sender_participant_id uuid NOT NULL REFERENCES session_participants (id),
    body                  text NOT NULL,
    sim_time              integer NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_session_channel ON messages (session_id, channel_id, sim_time);

CREATE TABLE decisions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        uuid NOT NULL REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    decision_point_id uuid NOT NULL REFERENCES decision_points (id),
    option_id         uuid NOT NULL REFERENCES decision_options (id),
    participant_id    uuid NOT NULL REFERENCES session_participants (id),
    team_id           uuid REFERENCES teams (id) ON DELETE SET NULL,
    rationale         text NOT NULL,
    confidence        smallint CHECK (confidence BETWEEN 1 AND 5),
    sim_time          integer NOT NULL,
    response_ms       bigint NOT NULL DEFAULT 0,
    snapshot          jsonb,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_decisions_session_participant ON decisions (session_id, participant_id, sim_time);

CREATE TABLE timeline_events (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id           uuid NOT NULL REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    seq                  bigint NOT NULL,
    sim_time             integer NOT NULL,
    type                 text NOT NULL,
    actor_participant_id uuid REFERENCES session_participants (id) ON DELETE SET NULL,
    team_id              uuid REFERENCES teams (id) ON DELETE SET NULL,
    visibility           text NOT NULL DEFAULT 'INSTRUCTOR' CHECK (visibility IN ('INSTRUCTOR', 'TEAM', 'PARTICIPANT', 'ALL')),
    payload              jsonb,
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, seq)
);

CREATE TABLE aar_reports (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id           uuid NOT NULL UNIQUE REFERENCES exercise_sessions (id) ON DELETE CASCADE,
    status               text NOT NULL DEFAULT 'GENERATING' CHECK (status IN ('GENERATING', 'READY', 'FAILED')),
    content              jsonb,
    lessons_learned      text NOT NULL DEFAULT '',
    released_to_trainees boolean NOT NULL DEFAULT false,
    generated_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
