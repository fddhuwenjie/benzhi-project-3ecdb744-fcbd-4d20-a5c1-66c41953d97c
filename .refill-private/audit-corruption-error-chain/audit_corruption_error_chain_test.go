package auditcorruptionerrorchain_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
)

func TestAuditCorruptionPreservesJSONErrorChain(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "drills.db")
	repository, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	drill := domain.DrillCase{
		ID: "drill-corrupt-audit", Title: "审计错误链复现", SiteName: "一号避难场所",
		ScenarioRisks: []string{"地震"}, PlannedStart: "2026-01-01T00:00:00Z",
		PlannedEnd: "2026-01-01T01:00:00Z", CoordinatorID: "coord",
		State: domain.StateDraft, RuleVersion: domain.CurrentRuleVersion, Revision: 1,
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}
	_, err = repository.Commit(context.Background(), store.CommitInput{
		Drill: drill, RequestID: "create-corrupt-audit", Fingerprint: "fingerprint",
		ActorID: "coord", EventType: "drill.created", EventPayload: map[string]any{"title": drill.Title},
		OccurredAt: drill.CreatedAt, Response: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE audit_events SET payload_json=? WHERE drill_id=? AND sequence=1`, []byte("{"), drill.ID); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(databasePath)
	if reopened != nil {
		reopened.Close()
	}
	if err == nil {
		t.Fatal("TestAuditCorruptionPreservesJSONErrorChain: 损坏的审计载荷未阻止重启")
	}
	var marshalError *json.MarshalerError
	if !errors.As(err, &marshalError) {
		t.Fatalf("TestAuditCorruptionPreservesJSONErrorChain: 底层 JSON 错误链丢失: %v", err)
	}
}
