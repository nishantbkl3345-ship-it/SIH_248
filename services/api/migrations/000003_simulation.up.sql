-- Simulation runtime. Simulation time is stored in milliseconds everywhere.

ALTER TABLE exercise_sessions RENAME COLUMN sim_time TO sim_ms;
ALTER TABLE exercise_sessions
    ALTER COLUMN sim_ms TYPE bigint,
    DROP COLUMN speed,
    -- Speed in thousandths (1000 = real time), matching the engine's integer clock.
    ADD COLUMN speed_milli integer NOT NULL DEFAULT 1000 CHECK (speed_milli BETWEEN 250 AND 20000);

ALTER TABLE teams
    ADD COLUMN number integer NOT NULL DEFAULT 1 CHECK (number BETWEEN 1 AND 8),
    ADD CONSTRAINT teams_session_number_key UNIQUE (session_id, number);

ALTER TABLE messages RENAME COLUMN sim_time TO sim_ms;
ALTER TABLE messages
    ALTER COLUMN sim_ms TYPE bigint,
    ADD COLUMN team_no integer NOT NULL DEFAULT 0;

ALTER TABLE decisions RENAME COLUMN sim_time TO sim_ms;
ALTER TABLE decisions
    ALTER COLUMN sim_ms TYPE bigint,
    DROP COLUMN team_id,
    ADD COLUMN team_no          integer NOT NULL DEFAULT 0,
    ADD COLUMN wall_time        timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN response_wall_ms bigint NOT NULL DEFAULT 0;

ALTER TABLE timeline_events RENAME COLUMN sim_time TO sim_ms;
ALTER TABLE timeline_events
    ALTER COLUMN sim_ms TYPE bigint,
    DROP COLUMN team_id,
    DROP COLUMN visibility,
    ADD COLUMN team_no   integer NOT NULL DEFAULT 0,
    ADD COLUMN wall_time timestamptz NOT NULL DEFAULT now();

-- The exercise record is append-only. Rows can be inserted and read, never
-- changed or removed; this also means a session with history cannot be
-- deleted, since that would cascade into this table.
CREATE FUNCTION timeline_events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'timeline_events is append-only' USING ERRCODE = 'restrict_violation';
END
$$;

CREATE TRIGGER timeline_events_no_update_or_delete
    BEFORE UPDATE OR DELETE ON timeline_events
    FOR EACH ROW EXECUTE FUNCTION timeline_events_append_only();

CREATE TRIGGER timeline_events_no_truncate
    BEFORE TRUNCATE ON timeline_events
    FOR EACH STATEMENT EXECUTE FUNCTION timeline_events_append_only();
