package integritymultidrillconnectiondeadlock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
)

func TestMultiDrillIntegrityCheckReleasesRowsBeforeTimeline(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	for _, id := range []string{"drill-a", "drill-b"} {
		drill := domain.DrillCase{
			ID:        id,
			Title:     id,
			SiteName:  "资源生命周期复现",
			State:     domain.StateDraft,
			Revision:  1,
			CreatedAt: "2026-08-27T00:00:00Z",
			UpdatedAt: "2026-08-27T00:00:00Z",
		}
		_, err := repository.Commit(context.Background(), store.CommitInput{
			Drill:        drill,
			RequestID:    "create-" + id,
			Fingerprint:  "fingerprint-" + id,
			ActorID:      "coordinator",
			EventType:    "drill.created",
			EventPayload: map[string]string{"drill_id": id},
			OccurredAt:   drill.CreatedAt,
			Response:     []byte(`{"drill_id":"` + id + `"}`),
		})
		if err != nil {
			t.Fatalf("准备 %s 失败: %v", id, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := repository.VerifyAllChains(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("多演练审计校验在持有 drill_id 结果集时等待唯一 SQLite 连接: %v", err)
		}
		t.Fatalf("多演练审计校验失败: %v", err)
	}
}
