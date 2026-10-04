ALTER TABLE decision_options ALTER COLUMN label DROP DEFAULT;

ALTER TABLE decision_points
    DROP COLUMN evaluation_criteria,
    DROP COLUMN ord,
    DROP COLUMN rationale_required,
    ALTER COLUMN time_limit_sec DROP NOT NULL,
    ALTER COLUMN time_limit_sec DROP DEFAULT,
    ALTER COLUMN prompt DROP DEFAULT,
    DROP CONSTRAINT decision_points_phase_id_fkey,
    ALTER COLUMN phase_id DROP NOT NULL,
    ADD CONSTRAINT decision_points_phase_id_fkey
        FOREIGN KEY (phase_id) REFERENCES scenario_phases (id) ON DELETE SET NULL;
ALTER TABLE decision_points RENAME COLUMN offset_sec TO opens_at_sim;

-- Rules in the new vocabulary cannot be expressed in the old one.
DELETE FROM degradation_rules;
ALTER TABLE degradation_rules
    DROP COLUMN target_team,
    DROP COLUMN ord,
    DROP COLUMN duration_sec,
    DROP COLUMN offset_sec,
    DROP COLUMN phase_id,
    ALTER COLUMN name DROP DEFAULT,
    ADD COLUMN from_sim integer,
    ADD COLUMN to_sim integer,
    ADD CHECK (from_sim IS NULL OR to_sim IS NULL OR to_sim >= from_sim),
    DROP CONSTRAINT degradation_rules_mode_check;
ALTER TABLE degradation_rules RENAME COLUMN mode TO effect;
ALTER TABLE degradation_rules
    ADD CONSTRAINT degradation_rules_effect_check
        CHECK (effect IN ('DELAY', 'DROP', 'PARTIAL', 'CONTRADICT', 'RELIABILITY_SHIFT'));

ALTER TABLE information_reports
    DROP COLUMN target_team,
    DROP COLUMN ord,
    DROP COLUMN priority,
    DROP COLUMN offset_sec,
    ALTER COLUMN title DROP DEFAULT,
    ALTER COLUMN source_name DROP DEFAULT,
    ADD COLUMN release_at_sim integer,
    DROP CONSTRAINT information_reports_phase_id_fkey,
    ALTER COLUMN phase_id DROP NOT NULL,
    ADD CONSTRAINT information_reports_phase_id_fkey
        FOREIGN KEY (phase_id) REFERENCES scenario_phases (id) ON DELETE SET NULL;

ALTER TABLE communication_channels
    DROP COLUMN ord,
    ALTER COLUMN name DROP DEFAULT;

ALTER TABLE scenario_phases
    DROP COLUMN duration_sec,
    ALTER COLUMN title DROP DEFAULT;
ALTER TABLE scenario_phases RENAME COLUMN description TO briefing;
ALTER TABLE scenario_phases RENAME COLUMN title TO name;

ALTER TABLE scenarios
    DROP COLUMN published_at,
    DROP COLUMN revision,
    DROP COLUMN trainee_count,
    DROP COLUMN team_count;
