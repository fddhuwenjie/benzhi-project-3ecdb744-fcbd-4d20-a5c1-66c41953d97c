package canceled_commit_persists_test

import (
	"context"
	"errors"
	"testing"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
)

func TestCanceledCommitMustNotPersist(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	drill := domain.DrillCase{
		ID: "drill-canceled", Title: "取消请求演练", SiteName: "一号避难场所",
		CoordinatorID: "coord", State: domain.StateDraft, RuleVersion: domain.CurrentRuleVersion,
		Revision: 1, CreatedAt: "2026-08-27T00:00:00Z", UpdatedAt: "2026-08-27T00:00:00Z",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, commitErr := repository.Commit(ctx, store.CommitInput{
		Drill: drill, RequestID: "request-canceled", Fingerprint: "fingerprint-canceled",
		ActorID: "coord", EventType: "drill.created", EventPayload: map[string]string{"title": drill.Title},
		OccurredAt: drill.CreatedAt, Response: []byte(`{"drill":{"id":"drill-canceled"}}`),
	})
	if !errors.Is(commitErr, context.Canceled) {
		persisted, loadErr := repository.Load(context.Background(), drill.ID)
		if loadErr == nil {
			t.Fatalf("TestCanceledCommitMustNotPersist: canceled transaction returned %v and persisted drill %s at revision %d", commitErr, persisted.ID, persisted.Revision)
		}
		t.Fatalf("TestCanceledCommitMustNotPersist: expected context.Canceled, got %v (load: %v)", commitErr, loadErr)
	}
	if _, loadErr := repository.Load(context.Background(), drill.ID); !errors.Is(loadErr, domain.ErrNotFound) {
		t.Fatalf("TestCanceledCommitMustNotPersist: canceled transaction left persisted aggregate: %v", loadErr)
	}
}
