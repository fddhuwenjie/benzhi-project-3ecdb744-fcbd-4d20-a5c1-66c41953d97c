package timeline_cache_stale_test

import (
	"context"
	"testing"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/workflow"
)

func TestTimelineCacheInvalidationAcrossMutation(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	service := workflow.New(repository)
	created, err := service.Create(context.Background(), workflow.CreateInput{
		RequestID:     "timeline-cache-create",
		ActorID:       "coordinator-a",
		Title:         "时间线缓存复现演练",
		SiteName:      "第一避难场所",
		ScenarioRisks: []string{"地震"},
		PlannedStart:  "2027-01-01T09:00:00Z",
		PlannedEnd:    "2027-01-01T10:00:00Z",
		Checkpoints: []domain.Checkpoint{{
			Sequence: 1, Name: "主入口集结", ResponsibleRole: "疏散引导员",
			TimeLimitSeconds: 180, MinimumCapacity: 20,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Timeline(context.Background(), created.Drill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Type != "drill.created" {
		t.Fatalf("unexpected initial timeline: %+v", first)
	}

	_, err = service.Freeze(context.Background(), created.Drill.ID, workflow.Mutation{
		RequestID:        "timeline-cache-freeze",
		ExpectedRevision: created.Drill.Revision,
		ActorID:          "coordinator-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Timeline(context.Background(), created.Drill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[1].Type != "baseline.frozen" {
		t.Fatalf("timeline remained stale after revision: got %+v", second)
	}
}
