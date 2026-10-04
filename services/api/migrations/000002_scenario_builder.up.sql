-- Scenario builder. Authored content now hangs off a phase and is timed as an
-- offset from the start of that phase, so resizing an earlier phase moves
-- later content with it instead of stranding it.

ALTER TABLE scenarios
    ADD COLUMN team_count    integer NOT NULL DEFAULT 2 CHECK (team_count BETWEEN 1 AND 8),
    ADD COLUMN trainee_count integer NOT NULL DEFAULT 8 CHECK (trainee_count BETWEEN 1 AND 60),
    -- Incremented on every save; lets the API reject a save based on stale data.
    ADD COLUMN revision      integer NOT NULL DEFAULT 1,
    ADD COLUMN published_at  timestamptz;

ALTER TABLE scenario_phases RENAME COLUMN name TO title;
ALTER TABLE scenario_phases RENAME COLUMN briefing TO description;
ALTER TABLE scenario_phases
    ALTER COLUMN title SET DEFAULT '',
    ADD COLUMN duration_sec integer NOT NULL DEFAULT 0 CHECK (duration_sec >= 0);

-- ord keeps children in the order the instructor arranged them.
ALTER TABLE communication_channels
    ALTER COLUMN name SET DEFAULT '',
    ADD COLUMN ord integer NOT NULL DEFAULT 0;

ALTER TABLE information_reports
    DROP CONSTRAINT information_reports_phase_id_fkey,
    ALTER COLUMN phase_id SET NOT NULL,
    ADD CONSTRAINT information_reports_phase_id_fkey
        FOREIGN KEY (phase_id) REFERENCES scenario_phases (id) ON DELETE CASCADE,
    DROP COLUMN release_at_sim,
    ALTER COLUMN source_name SET DEFAULT '',
    ALTER COLUMN title SET DEFAULT '',
    ADD COLUMN ord         integer NOT NULL DEFAULT 0,
    ADD COLUMN offset_sec  integer NOT NULL DEFAULT 0 CHECK (offset_sec >= 0),
    ADD COLUMN priority    text NOT NULL DEFAULT 'ROUTINE'
        CHECK (priority IN ('LOW', 'ROUTINE', 'HIGH', 'CRITICAL')),
    -- NULL addresses every team.
    ADD COLUMN target_team integer CHECK (target_team BETWEEN 1 AND 8);

ALTER TABLE degradation_rules RENAME COLUMN effect TO mode;
ALTER TABLE degradation_rules
    DROP CONSTRAINT degradation_rules_effect_check,
    ADD CONSTRAINT degradation_rules_mode_check
        CHECK (mode IN ('NORMAL', 'DELAY', 'DROPOUT', 'PARTIAL', 'CONFLICTING')),
    DROP COLUMN from_sim,
    DROP COLUMN to_sim,
    ALTER COLUMN name SET DEFAULT '',
    ADD COLUMN phase_id     uuid NOT NULL REFERENCES scenario_phases (id) ON DELETE CASCADE,
    ADD COLUMN ord          integer NOT NULL DEFAULT 0,
    ADD COLUMN offset_sec   integer NOT NULL DEFAULT 0 CHECK (offset_sec >= 0),
    ADD COLUMN duration_sec integer NOT NULL DEFAULT 0 CHECK (duration_sec >= 0),
    ADD COLUMN target_team  integer CHECK (target_team BETWEEN 1 AND 8);

ALTER TABLE decision_points RENAME COLUMN opens_at_sim TO offset_sec;
UPDATE decision_points SET time_limit_sec = 0 WHERE time_limit_sec IS NULL;
ALTER TABLE decision_points
    DROP CONSTRAINT decision_points_phase_id_fkey,
    ALTER COLUMN phase_id SET NOT NULL,
    ADD CONSTRAINT decision_points_phase_id_fkey
        FOREIGN KEY (phase_id) REFERENCES scenario_phases (id) ON DELETE CASCADE,
    ALTER COLUMN prompt SET DEFAULT '',
    ALTER COLUMN time_limit_sec SET DEFAULT 0,
    ALTER COLUMN time_limit_sec SET NOT NULL,
    ADD COLUMN ord                 integer NOT NULL DEFAULT 0,
    ADD COLUMN rationale_required  boolean NOT NULL DEFAULT true,
    ADD COLUMN evaluation_criteria text NOT NULL DEFAULT '';

ALTER TABLE decision_options ALTER COLUMN label SET DEFAULT '';
