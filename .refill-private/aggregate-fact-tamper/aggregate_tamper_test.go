package aggregatefacttamper

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/workflow"
)

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestOpenRejectsAggregateFactTampering(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/tampered.db"
	repository, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service := workflow.New(repository)
	now := time.Now().UTC().Truncate(time.Second)
	record := func(name string) domain.EvidenceRecord {
		return domain.EvidenceRecord{CollectedAt: now.Add(-time.Hour).Format(time.RFC3339), ValidUntil: now.Add(time.Hour).Format(time.RFC3339), ContentDigest: digest(name)}
	}
	view, err := service.Create(ctx, workflow.CreateInput{
		RequestID: "create", ActorID: "coord", Title: "完整性演练", SiteName: "一号场所",
		ScenarioRisks: []string{"地震"}, PlannedStart: now.Add(-30 * time.Minute).Format(time.RFC3339), PlannedEnd: now.Add(time.Hour).Format(time.RFC3339),
		Checkpoints: []domain.Checkpoint{{Sequence: 1, Name: "集结", ResponsibleRole: "引导岗", TimeLimitSeconds: 60, MinimumCapacity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Freeze(ctx, view.Drill.ID, workflow.Mutation{RequestID: "freeze", ExpectedRevision: view.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.RegisterActivation(ctx, view.Drill.ID, workflow.ActivationInput{
		Mutation:           workflow.Mutation{RequestID: "activation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		ActivationEvidence: domain.ActivationEvidence{EvacuationRoute: record("route"), CommunicationGear: record("radio"), AccessibleFacility: record("access"), PersonnelReady: record("people")},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Start(ctx, view.Drill.ID, workflow.Mutation{RequestID: "start", ExpectedRevision: view.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.SubmitObservation(ctx, view.Drill.ID, workflow.ObservationInput{
		Mutation: workflow.Mutation{RequestID: "observe", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, CheckpointID: view.Drill.Checkpoints[0].ID,
		ObservedAt: now.Format(time.RFC3339), ParticipantCount: 1, ElapsedSeconds: 30, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: digest("live"),
	})
	if err != nil {
		t.Fatal(err)
	}
	drillID := view.Drill.ID
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, `SELECT data_json FROM drills WHERE id=?`, drillID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var drill domain.DrillCase
	if err := json.Unmarshal(raw, &drill); err != nil {
		t.Fatal(err)
	}
	drill.Observations[0].ParticipantCount = 999
	raw, err = json.Marshal(drill)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE drills SET data_json=? WHERE id=?`, raw, drillID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(path)
	if err == nil {
		reopened.Close()
		t.Fatal("TestOpenRejectsAggregateFactTampering: startup accepted aggregate observations that disagree with the immutable observations table and audit chain")
	}
}
