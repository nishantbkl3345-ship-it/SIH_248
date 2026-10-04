DROP TRIGGER IF EXISTS timeline_events_no_truncate ON timeline_events;
DROP TRIGGER IF EXISTS timeline_events_no_update_or_delete ON timeline_events;
DROP FUNCTION IF EXISTS timeline_events_append_only();

ALTER TABLE timeline_events
    DROP COLUMN wall_time,
    DROP COLUMN team_no,
    ADD COLUMN visibility text NOT NULL DEFAULT 'INSTRUCTOR' CHECK (visibility IN ('INSTRUCTOR', 'TEAM', 'PARTICIPANT', 'ALL')),
    ADD COLUMN team_id uuid REFERENCES teams (id) ON DELETE SET NULL,
    ALTER COLUMN sim_ms TYPE integer;
ALTER TABLE timeline_events RENAME COLUMN sim_ms TO sim_time;

ALTER TABLE decisions
    DROP COLUMN response_wall_ms,
    DROP COLUMN wall_time,
    DROP COLUMN team_no,
    ADD COLUMN team_id uuid REFERENCES teams (id) ON DELETE SET NULL,
    ALTER COLUMN sim_ms TYPE integer;
ALTER TABLE decisions RENAME COLUMN sim_ms TO sim_time;

ALTER TABLE messages
    DROP COLUMN team_no,
    ALTER COLUMN sim_ms TYPE integer;
ALTER TABLE messages RENAME COLUMN sim_ms TO sim_time;

ALTER TABLE teams
    DROP CONSTRAINT teams_session_number_key,
    DROP COLUMN number;

ALTER TABLE exercise_sessions
    DROP COLUMN speed_milli,
    ADD COLUMN speed double precision NOT NULL DEFAULT 1 CHECK (speed > 0),
    ALTER COLUMN sim_ms TYPE integer;
ALTER TABLE exercise_sessions RENAME COLUMN sim_ms TO sim_time;
