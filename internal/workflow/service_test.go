package workflow

import (
	"context"
	"testing"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return New(repository)
}

func TestCreateAndReplayAreIdempotent(t *testing.T) {
	service := testService(t)
	input := CreateInput{RequestID: "same", ActorID: "coord", Title: "演练", SiteName: "场所", ScenarioRisks: []string{"地震"}, PlannedStart: "2026-01-01T00:00:00Z", PlannedEnd: "2026-01-01T01:00:00Z", Checkpoints: []domain.Checkpoint{{Sequence: 1, Name: "集结", ResponsibleRole: "岗", TimeLimitSeconds: 60, MinimumCapacity: 1}}}
	first, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.Drill.ID != first.Drill.ID {
		t.Fatalf("unexpected replay: %+v", second)
	}
}

func TestRevisionConflict(t *testing.T) {
	service := testService(t)
	input := CreateInput{RequestID: "create", ActorID: "coord", Title: "演练", SiteName: "场所", ScenarioRisks: []string{"地震"}, PlannedStart: "2026-01-01T00:00:00Z", PlannedEnd: "2026-01-01T01:00:00Z", Checkpoints: []domain.Checkpoint{{Sequence: 1, Name: "集结", ResponsibleRole: "岗", TimeLimitSeconds: 60, MinimumCapacity: 1}}}
	view, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Freeze(context.Background(), view.Drill.ID, Mutation{RequestID: "freeze", ActorID: "coord", ExpectedRevision: view.Drill.Revision + 1})
	if _, ok := err.(*domain.RevisionConflict); !ok {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

func TestReviewRequiresRoleSeparation(t *testing.T) {
	if err := domain.ValidateReviewSeparation("coord", "coord"); err != domain.ErrForbidden {
		t.Fatalf("expected separation error, got %v", err)
	}
}
