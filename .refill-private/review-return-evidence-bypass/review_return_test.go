package reviewreturnevidencebypass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestEvidenceReturnCannotCloseWithOrphanRetest(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := workflow.New(repository)
	now := time.Now().UTC().Truncate(time.Second)
	record := func(name string) domain.EvidenceRecord {
		return domain.EvidenceRecord{CollectedAt: now.Add(-time.Hour).Format(time.RFC3339), ValidUntil: now.Add(time.Hour).Format(time.RFC3339), ContentDigest: digest(name)}
	}

	view, err := service.Create(ctx, workflow.CreateInput{
		RequestID: "create", ActorID: "coord", Title: "证据退回演练", SiteName: "一号场所",
		ScenarioRisks: []string{"地震"}, PlannedStart: now.Add(-30 * time.Minute).Format(time.RFC3339),
		PlannedEnd:  now.Add(2 * time.Hour).Format(time.RFC3339),
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
		Mutation: workflow.Mutation{RequestID: "activation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		ActivationEvidence: domain.ActivationEvidence{
			EvacuationRoute: record("route"), CommunicationGear: record("radio"),
			AccessibleFacility: record("access"), PersonnelReady: record("people"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Start(ctx, view.Drill.ID, workflow.Mutation{RequestID: "start", ExpectedRevision: view.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.SubmitObservation(ctx, view.Drill.ID, workflow.ObservationInput{
		Mutation:     workflow.Mutation{RequestID: "observe", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		CheckpointID: view.Drill.Checkpoints[0].ID, ObservedAt: now.Format(time.RFC3339), ParticipantCount: 1,
		ElapsedSeconds: 30, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: digest("live"),
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.SubmitReview(ctx, view.Drill.ID, workflow.Mutation{RequestID: "submit", ExpectedRevision: view.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	results := make([]domain.ReviewChecklistResult, 0, len(view.Drill.CurrentSnapshot.Checklist))
	for _, item := range view.Drill.CurrentSnapshot.Checklist {
		conclusion, opinion := "passed", "核验通过"
		if item.ID == "activation:communication_gear" {
			conclusion, opinion = "returned", "通信设备证据无法核实"
		}
		results = append(results, domain.ReviewChecklistResult{ItemID: item.ID, Conclusion: conclusion, Opinion: opinion})
	}
	view, err = service.ReviewDecision(ctx, view.Drill.ID, workflow.ReviewDecisionInput{
		Mutation: workflow.Mutation{RequestID: "return", ExpectedRevision: view.Drill.Revision, ActorID: "reviewer"},
		Decision: "returned", Reason: "证据需重新采集", SnapshotDigest: view.Drill.CurrentSnapshot.PayloadDigest, Checklist: results,
	})
	if err != nil {
		t.Fatal(err)
	}
	finding := view.Drill.Findings[len(view.Drill.Findings)-1]
	if finding.EvidenceCategory != "communication_gear" || finding.CheckpointID != "" {
		t.Fatalf("unexpected directed finding: %+v", finding)
	}
	view, err = service.PlanRemediation(ctx, view.Drill.ID, finding.ID, workflow.RemediationInput{
		Mutation: workflow.Mutation{RequestID: "plan", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		Cause:    "原设备日志缺失", CorrectiveAction: "重新采集通信设备日志", OwnerID: "owner",
		RetestPlannedAt: now.Add(30 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitRetest(ctx, view.Drill.ID, finding.ID, workflow.RetestInput{
		Mutation:   workflow.Mutation{RequestID: "orphan-retest", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		ObservedAt: now.Add(5 * time.Minute).Format(time.RFC3339), ParticipantCount: 0, ElapsedSeconds: 0,
		CommunicationPassed: false, AccessibilityPassed: false, EvidenceDigest: digest("unrelated-retest"),
	})
	if err == nil {
		t.Fatal("TestEvidenceReturnCannotCloseWithOrphanRetest: evidence-category return closed through a retest with no checkpoint or replacement activation evidence")
	}
}
