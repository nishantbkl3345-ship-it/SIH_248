package gormrepo_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fogline/api/internal/domain"
	"fogline/api/internal/platform/database"
	"fogline/api/internal/repository"
	"fogline/api/internal/repository/gormrepo"
)

// These tests need a real, disposable PostgreSQL database. They drop and
// recreate every table, so point TEST_DATABASE_URL at a database used for
// nothing else. Without it they are skipped.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration tests")
	}
	if err := database.MigrateDown(url); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	if err := database.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := database.Open(context.Background(), url, true)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func TestMigrationsAreReversibleAndIdempotent(t *testing.T) {
	openTestDB(t)
	url := os.Getenv("TEST_DATABASE_URL")

	if err := database.MigrateUp(url); err != nil {
		t.Fatalf("second MigrateUp should be a no-op: %v", err)
	}
	if err := database.MigrateDown(url); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	if err := database.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp after down: %v", err)
	}
}

func TestUserRepository(t *testing.T) {
	db := openTestDB(t)
	repo := gormrepo.NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{
		Email: "mira@example.test", PasswordHash: "hash", DisplayName: "Mira",
		Role: domain.RoleTrainee, IsActive: true,
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.ID.String() == "00000000-0000-0000-0000-000000000000" || user.CreatedAt.IsZero() {
		t.Fatalf("database defaults not read back: %+v", user)
	}

	dup := &domain.User{Email: "mira@example.test", PasswordHash: "h", DisplayName: "D", Role: domain.RoleTrainee}
	if err := repo.Create(ctx, dup); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplicate email error = %v, want ErrEmailTaken", err)
	}

	got, err := repo.FindByEmail(ctx, "mira@example.test")
	if err != nil || got.ID != user.ID || !got.IsActive {
		t.Fatalf("FindByEmail: %+v, %v", got, err)
	}
	if _, err := repo.FindByEmail(ctx, "nobody@example.test"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing user error = %v, want ErrNotFound", err)
	}

	// A false boolean must be written, not replaced by the column default.
	got.IsActive = false
	got.Role = domain.RoleInstructor
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.TouchLastLogin(ctx, got.ID, now); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}
	got, err = repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.IsActive || got.Role != domain.RoleInstructor || got.PasswordHash != "hash" {
		t.Fatalf("after update: %+v", got)
	}
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(now) {
		t.Fatalf("LastLoginAt = %v, want %v", got.LastLoginAt, now)
	}

	users, total, err := repo.List(ctx, 10, 0)
	if err != nil || total != 1 || len(users) != 1 {
		t.Fatalf("List: len=%d total=%d err=%v", len(users), total, err)
	}

	bad := &domain.User{Email: "x@example.test", PasswordHash: "h", DisplayName: "X", Role: "ROOT"}
	if err := repo.Create(ctx, bad); err == nil {
		t.Fatal("role CHECK constraint did not reject an unknown role")
	}
}

// TestModelsMatchSchema writes and reads one row of every model so a drift
// between the Go structs and the SQL migrations fails here, not in production.
func TestModelsMatchSchema(t *testing.T) {
	db := openTestDB(t).WithContext(context.Background())
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	intp := func(i int) *int { return &i }

	instructor := domain.User{Email: "i@example.test", PasswordHash: "h", DisplayName: "I", Role: domain.RoleInstructor, IsActive: true}
	trainee := domain.User{Email: "t@example.test", PasswordHash: "h", DisplayName: "T", Role: domain.RoleTrainee, IsActive: true}
	create(&instructor)
	create(&trainee)

	scenario := domain.Scenario{
		OwnerID: instructor.ID, Title: "Exercise Tidewatch", Summary: "Fictional relief routing",
		Objectives: domain.JSONB(`["Choose a route under uncertainty"]`), Difficulty: domain.DifficultyIntermediate,
		Status: domain.ScenarioDraft, EstDurationSec: 600, TeamCount: 2, TraineeCount: 8, Revision: 1,
	}
	create(&scenario)
	phase := domain.ScenarioPhase{ScenarioID: scenario.ID, Ord: 1, Title: "Assessment", Description: "A storm has passed.", DurationSec: 600}
	create(&phase)

	channel := domain.CommunicationChannel{ScenarioID: scenario.ID, Key: "team-net", Name: "Team Net", Kind: domain.ChannelTeam}
	create(&channel)

	create(&domain.InformationReport{
		ScenarioID: scenario.ID, PhaseID: phase.ID, ChannelID: &channel.ID,
		SourceName: "Relay Station K-7", SourceReliability: "A",
		Title: "Coastal road", Body: "Passable.", OffsetSec: 120, Priority: domain.PriorityHigh, TargetTeam: intp(1),
	})
	create(&domain.ScenarioEvent{
		ScenarioID: scenario.ID, PhaseID: &phase.ID, Name: "Field feed slows",
		TriggerKind: domain.TriggerScheduled, AtSim: intp(150), Actions: domain.JSONB(`[]`),
	})
	create(&domain.DegradationRule{
		ScenarioID: scenario.ID, PhaseID: phase.ID, ChannelID: &channel.ID, Name: "30s delay", Mode: domain.ModeDelay,
		OffsetSec: 150, DurationSec: 60, Params: domain.JSONB(`{"delaySec":30}`),
	})

	point := domain.DecisionPoint{
		ScenarioID: scenario.ID, PhaseID: phase.ID, Scope: domain.ScopeTeam,
		Prompt: "Which route?", OffsetSec: 260, TimeLimitSec: 120,
	}
	create(&point)
	options := []domain.DecisionOption{
		{DecisionPointID: point.ID, Ord: 1, Label: "Coastal road"},
		{DecisionPointID: point.ID, Ord: 2, Label: "Inland pass"},
	}
	create(&options)
	point.Options = options

	session := domain.ExerciseSession{
		ScenarioID: scenario.ID, InstructorID: instructor.ID, Name: "Demo run", JoinCode: "ABC123",
		Seed: 42, Status: domain.SessionLobby, SpeedMilli: 1000,
	}
	create(&session)
	team := domain.Team{SessionID: session.ID, Number: 1, Name: "Amber", Color: "#f59e0b"}
	create(&team)
	participant := domain.SessionParticipant{SessionID: session.ID, UserID: trainee.ID, TeamID: &team.ID, Seat: "LEAD"}
	create(&participant)
	if participant.JoinedAt.IsZero() {
		t.Error("JoinedAt was not set")
	}

	create(&domain.Message{SessionID: session.ID, ChannelID: channel.ID, SenderParticipantID: participant.ID, TeamNo: 1, Body: "Copy.", SimMs: 130_500})
	confidence := int16(4)
	create(&domain.Decision{
		SessionID: session.ID, DecisionPointID: point.ID, OptionID: point.Options[0].ID,
		ParticipantID: participant.ID, TeamNo: 1, Rationale: "Only confirmed route.",
		Confidence: &confidence, SimMs: 300_250, WallTime: time.Now(), ResponseMs: 40_000, ResponseWallMs: 8_000,
		Snapshot: domain.JSONB(`{"reports":[]}`),
	})
	create(&domain.TimelineEvent{
		SessionID: session.ID, Seq: 1, SimMs: 0, WallTime: time.Now(), Type: "EXERCISE_STARTED",
		ActorParticipantID: &participant.ID, TeamNo: 1,
	})
	create(&domain.AARReport{SessionID: session.ID, Status: domain.AARGenerating})

	// Read back through the repository to check column mapping both ways.
	loaded, err := gormrepo.NewScenarioRepository(db).Get(context.Background(), scenario.ID)
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}
	if len(loaded.Phases) != 1 || len(loaded.Channels) != 1 || string(loaded.Objectives) == "" {
		t.Fatalf("scenario round trip: %+v", loaded)
	}
	p := loaded.Phases[0]
	if len(p.Reports) != 1 || len(p.Rules) != 1 || len(p.DecisionPoints) != 1 || len(p.DecisionPoints[0].Options) != 2 {
		t.Fatalf("phase children round trip: %+v", p)
	}
	if r := p.Reports[0]; r.OffsetSec != 120 || r.Priority != domain.PriorityHigh || r.TargetTeam == nil || *r.TargetTeam != 1 {
		t.Errorf("report round trip: %+v", r)
	}
	if r := p.Rules[0]; r.Mode != domain.ModeDelay || r.DurationSec != 60 || string(r.Params) == "" {
		t.Errorf("rule round trip: %+v", r)
	}
	var event domain.TimelineEvent
	if err := db.First(&event, "session_id = ?", session.ID).Error; err != nil {
		t.Fatalf("load timeline event: %v", err)
	}
	if event.Payload != nil {
		t.Errorf("empty JSONB should round-trip as nil, got %q", event.Payload)
	}
	var report domain.AARReport
	if err := db.First(&report, "session_id = ?", session.ID).Error; err != nil {
		t.Fatalf("load AAR report: %v", err)
	}

	// The (session_id, seq) uniqueness that orders the timeline must hold.
	dupSeq := domain.TimelineEvent{SessionID: session.ID, Seq: 1, Type: "X", WallTime: time.Now()}
	if err := db.Create(&dupSeq).Error; err == nil {
		t.Error("duplicate timeline seq was accepted")
	}
}

func newScenario(owner uuid.UUID) *domain.Scenario {
	id, phase, channel, point := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	return &domain.Scenario{
		ID: id, OwnerID: owner, Title: "Exercise Tidewatch", Difficulty: domain.DifficultyBasic,
		Status: domain.ScenarioDraft, TeamCount: 2, TraineeCount: 6, Revision: 1, Objectives: domain.JSONB(`["a"]`),
		Channels: []domain.CommunicationChannel{{ID: channel, ScenarioID: id, Key: "feed", Name: "Field Feed", Kind: domain.ChannelFeed}},
		Phases: []domain.ScenarioPhase{{
			ID: phase, ScenarioID: id, Ord: 1, Title: "Assessment", DurationSec: 300,
			Reports: []domain.InformationReport{{
				ID: uuid.New(), ScenarioID: id, PhaseID: phase, ChannelID: &channel, SourceName: "K-7",
				SourceReliability: "B", Title: "Road", Body: "Open.", OffsetSec: 60, Priority: domain.PriorityRoutine,
			}},
			Rules: []domain.DegradationRule{{
				ID: uuid.New(), ScenarioID: id, PhaseID: phase, ChannelID: &channel, Name: "Down",
				Mode: domain.ModeDropout, OffsetSec: 90, DurationSec: 30,
			}},
			DecisionPoints: []domain.DecisionPoint{{
				ID: point, ScenarioID: id, PhaseID: phase, Scope: domain.ScopeIndividual, Prompt: "Go?", OffsetSec: 120, TimeLimitSec: 60,
				Options: []domain.DecisionOption{
					{ID: uuid.New(), DecisionPointID: point, Ord: 1, Label: "Yes"},
					{ID: uuid.New(), DecisionPointID: point, Ord: 2, Label: "No"},
				},
			}},
		}},
	}
}

func counts(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) (phases, reports, rules, points, channels int64) {
	t.Helper()
	db.Model(&domain.ScenarioPhase{}).Where("scenario_id = ?", scenarioID).Count(&phases)
	db.Model(&domain.InformationReport{}).Where("scenario_id = ?", scenarioID).Count(&reports)
	db.Model(&domain.DegradationRule{}).Where("scenario_id = ?", scenarioID).Count(&rules)
	db.Model(&domain.DecisionPoint{}).Where("scenario_id = ?", scenarioID).Count(&points)
	db.Model(&domain.CommunicationChannel{}).Where("scenario_id = ?", scenarioID).Count(&channels)
	return
}

func TestScenarioRepository(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := gormrepo.NewScenarioRepository(db)

	owner := domain.User{Email: "i@example.test", PasswordHash: "h", DisplayName: "I", Role: domain.RoleInstructor, IsActive: true}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}

	first := newScenario(owner.ID)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Channels) != 1 || len(got.Phases) != 1 || len(got.Phases[0].Reports) != 1 ||
		len(got.Phases[0].Rules) != 1 || len(got.Phases[0].DecisionPoints[0].Options) != 2 {
		t.Fatalf("created aggregate incomplete: %+v", got)
	}

	// Replace swaps the whole content: a second phase appears, the rule goes.
	edit := newScenario(owner.ID)
	edit.ID = first.ID
	edit.Title = "Renamed"
	edit.EstDurationSec = 480
	for i := range edit.Channels {
		edit.Channels[i].ScenarioID = first.ID
	}
	edit.Phases[0].ScenarioID = first.ID
	edit.Phases[0].Rules = nil
	for i := range edit.Phases[0].Reports {
		edit.Phases[0].Reports[i].ScenarioID = first.ID
	}
	for i := range edit.Phases[0].DecisionPoints {
		edit.Phases[0].DecisionPoints[i].ScenarioID = first.ID
	}
	edit.Phases = append(edit.Phases, domain.ScenarioPhase{ID: uuid.New(), ScenarioID: first.ID, Ord: 2, Title: "Commitment", DurationSec: 180, StartsAtSim: 300})
	if err := repo.Replace(ctx, edit, 1); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, _ = repo.Get(ctx, first.ID)
	if got.Title != "Renamed" || got.Revision != 2 || got.EstDurationSec != 480 || len(got.Phases) != 2 || got.Phases[1].Title != "Commitment" {
		t.Fatalf("after replace: %+v", got)
	}
	if phases, reports, rules, points, channels := counts(t, db, first.ID); phases != 2 || reports != 1 || rules != 0 || points != 1 || channels != 1 {
		t.Fatalf("row counts after replace: phases=%d reports=%d rules=%d points=%d channels=%d", phases, reports, rules, points, channels)
	}

	if err := repo.Replace(ctx, edit, 1); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v, want ErrRevisionConflict", err)
	}

	// A caller-supplied id that belongs to another scenario must be refused,
	// must not modify that scenario, and must roll the whole save back.
	second := newScenario(owner.ID)
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	hijack := newScenario(owner.ID)
	hijack.ID = first.ID
	hijack.Title = "Should not be saved"
	hijack.Channels = nil
	hijack.Phases = []domain.ScenarioPhase{{ID: second.Phases[0].ID, ScenarioID: first.ID, Ord: 1, Title: "Stolen", DurationSec: 1}}
	if err := repo.Replace(ctx, hijack, 2); !errors.Is(err, domain.ErrIDConflict) {
		t.Fatalf("hijack error = %v, want ErrIDConflict", err)
	}
	victim, _ := repo.Get(ctx, second.ID)
	if len(victim.Phases) != 1 || victim.Phases[0].Title != "Assessment" || victim.Phases[0].ScenarioID != second.ID {
		t.Fatalf("another scenario's phase was modified: %+v", victim.Phases)
	}
	got, _ = repo.Get(ctx, first.ID)
	if got.Title != "Renamed" || got.Revision != 2 || len(got.Phases) != 2 {
		t.Fatalf("failed save was not rolled back: %+v", got)
	}

	// Status transitions bump the revision and only apply from the expected status.
	now := time.Now()
	if err := repo.SetStatus(ctx, first.ID, domain.ScenarioDraft, domain.ScenarioPublished, &now); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if err := repo.SetStatus(ctx, first.ID, domain.ScenarioDraft, domain.ScenarioPublished, &now); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("second publish error = %v, want ErrRevisionConflict", err)
	}
	if err := repo.Replace(ctx, edit, 3); !errors.Is(err, domain.ErrNotDraft) {
		t.Fatalf("replace on published error = %v, want ErrNotDraft", err)
	}

	list, err := repo.List(ctx, &owner.ID)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || len(list[0].Phases) != 2 {
		t.Fatalf("List: %+v, %v", list, err)
	}
	stranger := uuid.New()
	if list, _ := repo.List(ctx, &stranger); len(list) != 0 {
		t.Fatalf("stranger sees %d scenarios", len(list))
	}

	if n, err := repo.CountSessions(ctx, first.ID); err != nil || n != 0 {
		t.Fatalf("CountSessions = %d, %v", n, err)
	}
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if phases, reports, _, points, channels := counts(t, db, first.ID); phases+reports+points+channels != 0 {
		t.Fatal("delete left child rows behind")
	}
	if err := repo.Delete(ctx, first.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepository(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	scenarios := gormrepo.NewScenarioRepository(db)
	repo := gormrepo.NewSessionRepository(db)

	instructor := domain.User{Email: "i@example.test", PasswordHash: "h", DisplayName: "Instructor", Role: domain.RoleInstructor, IsActive: true}
	alice := domain.User{Email: "a@example.test", PasswordHash: "h", DisplayName: "Alice", Role: domain.RoleTrainee, IsActive: true}
	bela := domain.User{Email: "b@example.test", PasswordHash: "h", DisplayName: "Bela", Role: domain.RoleTrainee, IsActive: true}
	for _, u := range []*domain.User{&instructor, &alice, &bela} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	sc := newScenario(instructor.ID)
	if err := scenarios.Create(ctx, sc); err != nil {
		t.Fatal(err)
	}

	sess := &domain.ExerciseSession{
		ScenarioID: sc.ID, InstructorID: instructor.ID, Name: "Run", JoinCode: "QWERTY", Seed: 7,
		Status: domain.SessionLobby, SpeedMilli: 1000,
		Teams: []domain.Team{{Number: 2, Name: "Team 2"}, {Number: 1, Name: "Team 1"}},
	}
	if err := repo.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dup := &domain.ExerciseSession{ScenarioID: sc.ID, InstructorID: instructor.ID, Name: "Dup", JoinCode: "QWERTY", Status: domain.SessionLobby, SpeedMilli: 1000}
	if err := repo.Create(ctx, dup); !errors.Is(err, domain.ErrJoinCodeTaken) {
		t.Fatalf("duplicate join code error = %v, want ErrJoinCodeTaken", err)
	}

	got, err := repo.FindByJoinCode(ctx, "QWERTY")
	if err != nil || got.ID != sess.ID || len(got.Teams) != 2 || got.Teams[0].Number != 1 {
		t.Fatalf("FindByJoinCode: %+v, %v", got, err)
	}
	if _, err := repo.FindByJoinCode(ctx, "NOPE00"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown code error = %v", err)
	}

	pa := &domain.SessionParticipant{SessionID: sess.ID, UserID: alice.ID, TeamID: &got.Teams[0].ID}
	if err := repo.AddParticipant(ctx, pa); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}
	again := &domain.SessionParticipant{SessionID: sess.ID, UserID: alice.ID}
	if err := repo.AddParticipant(ctx, again); !errors.Is(err, domain.ErrAlreadyJoined) {
		t.Fatalf("repeat join error = %v, want ErrAlreadyJoined", err)
	}
	pb := &domain.SessionParticipant{SessionID: sess.ID, UserID: bela.ID, TeamID: &got.Teams[0].ID}
	if err := repo.AddParticipant(ctx, pb); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetParticipantTeam(ctx, pb.ID, got.Teams[1].ID); err != nil {
		t.Fatalf("SetParticipantTeam: %v", err)
	}
	if err := repo.SetParticipantTeam(ctx, uuid.New(), got.Teams[1].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown participant error = %v", err)
	}

	got, _ = repo.Get(ctx, sess.ID)
	if len(got.Participants) != 2 || got.Participants[0].User == nil || got.Participants[0].User.DisplayName != "Alice" ||
		*got.Participants[1].TeamID != got.Teams[1].ID {
		t.Fatalf("participants: %+v", got.Participants)
	}

	// Lists are scoped.
	if l, _ := repo.ListForInstructor(ctx, &instructor.ID); len(l) != 1 {
		t.Errorf("instructor sees %d sessions", len(l))
	}
	if l, _ := repo.ListForInstructor(ctx, &alice.ID); len(l) != 0 {
		t.Errorf("a non-instructor id matched %d sessions", len(l))
	}
	if l, _ := repo.ListForUser(ctx, bela.ID); len(l) != 1 {
		t.Errorf("bela sees %d sessions", len(l))
	}
	if l, _ := repo.ListForUser(ctx, instructor.ID); len(l) != 0 {
		t.Errorf("the instructor is listed as a participant in %d sessions", len(l))
	}

	// Progress: started_at and ended_at are written once and kept.
	started := time.Now().UTC().Truncate(time.Millisecond)
	later := started.Add(time.Hour)
	if err := repo.UpdateProgress(ctx, sess.ID, repository.SessionProgress{Status: domain.SessionRunning, SimMs: 1500, SpeedMilli: 5000, StartedAt: &started}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if err := repo.UpdateProgress(ctx, sess.ID, repository.SessionProgress{Status: domain.SessionRunning, SimMs: 9000, SpeedMilli: 5000, StartedAt: &later}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, sess.ID)
	if got.Status != domain.SessionRunning || got.SimMs != 9000 || got.SpeedMilli != 5000 || !got.StartedAt.Equal(started) || got.EndedAt != nil {
		t.Fatalf("after progress: %+v", got)
	}

	// History is stored atomically: a bad row in the batch stores nothing.
	channel := sc.Channels[0].ID
	point := sc.Phases[0].DecisionPoints[0]
	event := func(seq int64) domain.TimelineEvent {
		return domain.TimelineEvent{SessionID: sess.ID, Seq: seq, SimMs: seq * 1000, WallTime: started, Type: "REPORT_DELIVERED", TeamNo: 1, Payload: domain.JSONB(`{"n":1}`)}
	}
	good := repository.SessionRecords{
		Events:   []domain.TimelineEvent{event(1), event(2)},
		Messages: []domain.Message{{ID: uuid.New(), SessionID: sess.ID, ChannelID: channel, SenderParticipantID: pa.ID, TeamNo: 1, Body: "hello", SimMs: 1200}},
		Decisions: []domain.Decision{{
			ID: uuid.New(), SessionID: sess.ID, DecisionPointID: point.ID, OptionID: point.Options[0].ID, ParticipantID: pa.ID,
			TeamNo: 1, Rationale: "r", SimMs: 1900, WallTime: started, ResponseMs: 700, ResponseWallMs: 140, Snapshot: domain.JSONB(`{}`),
		}},
	}
	if err := repo.AppendRecords(ctx, good); err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}
	bad := repository.SessionRecords{
		Messages: []domain.Message{{ID: uuid.New(), SessionID: sess.ID, ChannelID: channel, SenderParticipantID: pa.ID, Body: "should roll back", SimMs: 3000}},
		Events:   []domain.TimelineEvent{event(3), event(2)}, // seq 2 already exists
	}
	if err := repo.AppendRecords(ctx, bad); err == nil {
		t.Fatal("a batch with a duplicate sequence number was stored")
	}
	var messages int64
	db.Model(&domain.Message{}).Where("session_id = ?", sess.ID).Count(&messages)
	events, err := repo.Timeline(ctx, sess.ID, 0, 100)
	if err != nil || len(events) != 2 || messages != 1 {
		t.Fatalf("after a failed batch: %d events, %d messages, err=%v; want 2 and 1", len(events), messages, err)
	}
	if after, _ := repo.Timeline(ctx, sess.ID, 1, 100); len(after) != 1 || after[0].Seq != 2 || string(after[0].Payload) != `{"n": 1}` && string(after[0].Payload) != `{"n":1}` {
		t.Fatalf("Timeline(afterSeq=1) = %+v", after)
	}

	// The timeline cannot be rewritten or erased, by anyone, through SQL.
	for name, stmt := range map[string]string{
		"update":   "UPDATE timeline_events SET type = 'TAMPERED' WHERE session_id = ?",
		"delete":   "DELETE FROM timeline_events WHERE session_id = ?",
		"truncate": "TRUNCATE timeline_events",
	} {
		args := []any{sess.ID}
		if name == "truncate" {
			args = nil
		}
		if err := db.Exec(stmt, args...).Error; err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s on timeline_events: err = %v, want an append-only refusal", name, err)
		}
	}
	if err := repo.Delete(ctx, sess.ID); err == nil {
		t.Error("a session with recorded history was deleted")
	}
	events, _ = repo.Timeline(ctx, sess.ID, 0, 100)
	if len(events) != 2 || events[0].Type != "REPORT_DELIVERED" {
		t.Fatalf("timeline changed despite the refusals: %+v", events)
	}

	// A scenario that has been run is counted as in use.
	if n, _ := scenarios.CountSessions(ctx, sc.ID); n != 1 {
		t.Errorf("CountSessions = %d, want 1", n)
	}

	// Sessions left live by a crash are closed at startup; lobbies are left alone.
	lobby := &domain.ExerciseSession{ScenarioID: sc.ID, InstructorID: instructor.ID, Name: "Lobby", JoinCode: "LOBBY1", Status: domain.SessionLobby, SpeedMilli: 1000}
	if err := repo.Create(ctx, lobby); err != nil {
		t.Fatal(err)
	}
	n, err := repo.InterruptLive(ctx, later)
	if err != nil || n != 1 {
		t.Fatalf("InterruptLive = %d, %v; want 1", n, err)
	}
	got, _ = repo.Get(ctx, sess.ID)
	stillLobby, _ := repo.Get(ctx, lobby.ID)
	if got.Status != domain.SessionEnded || got.EndedAt == nil || stillLobby.Status != domain.SessionLobby {
		t.Errorf("after InterruptLive: %s / %s", got.Status, stillLobby.Status)
	}
	// A lobby with no history can be deleted.
	if err := repo.Delete(ctx, lobby.ID); err != nil {
		t.Errorf("deleting an unused lobby: %v", err)
	}
}
