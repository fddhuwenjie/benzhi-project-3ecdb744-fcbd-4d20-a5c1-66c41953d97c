package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"shelter-drill-gate/internal/domain"
)

var featureNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func featureDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func featureRecord(source string, validUntil time.Time) domain.EvidenceRecord {
	return domain.EvidenceRecord{CollectedAt: featureNow.Add(-time.Minute).Format(time.RFC3339), ValidUntil: validUntil.Format(time.RFC3339), ContentDigest: featureDigest(source)}
}

func featureCreate(t *testing.T, service *Service, checkpoints []domain.Checkpoint) DrillView {
	t.Helper()
	view, err := service.Create(context.Background(), CreateInput{
		RequestID: "feature-create", ActorID: "coord", Title: "扩展演练", SiteName: "一号场所",
		ScenarioRisks: []string{"地震"}, PlannedStart: featureNow.Add(-10 * time.Minute).Format(time.RFC3339),
		PlannedEnd: featureNow.Add(2 * time.Hour).Format(time.RFC3339), Checkpoints: checkpoints,
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Freeze(context.Background(), view.Drill.ID, Mutation{RequestID: "feature-freeze", ExpectedRevision: view.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func featureActivation(validUntil time.Time) domain.ActivationEvidence {
	return domain.ActivationEvidence{
		EvacuationRoute: featureRecord("route", validUntil), CommunicationGear: featureRecord("radio", validUntil),
		AccessibleFacility: featureRecord("ramp", validUntil), PersonnelReady: featureRecord("roster", validUntil),
	}
}

func featureStart(t *testing.T, service *Service, checkpoints []domain.Checkpoint) DrillView {
	t.Helper()
	view := featureCreate(t, service, checkpoints)
	activation, err := service.RegisterActivation(context.Background(), view.Drill.ID, ActivationInput{
		Mutation:           Mutation{RequestID: "feature-activation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"},
		ActivationEvidence: featureActivation(featureNow.Add(time.Hour)),
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.Start(context.Background(), view.Drill.ID, Mutation{RequestID: "feature-start", ExpectedRevision: activation.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func TestActivationEvidenceIntegrityAndExpiryGate(t *testing.T) {
	service := testService(t)
	service.now = func() time.Time { return featureNow }
	view := featureCreate(t, service, []domain.Checkpoint{{Sequence: 1, Name: "集结", ResponsibleRole: "疏散岗", TimeLimitSeconds: 120, MinimumCapacity: 30}})

	invalid := featureActivation(featureNow.Add(time.Hour))
	invalid.CommunicationGear.ContentDigest = invalid.EvacuationRoute.ContentDigest
	_, err := service.RegisterActivation(context.Background(), view.Drill.ID, ActivationInput{Mutation: Mutation{RequestID: "invalid-activation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, ActivationEvidence: invalid})
	if !domain.IsValidation(err) {
		t.Fatalf("expected duplicate digest validation, got %v", err)
	}
	unchanged, _ := service.Get(context.Background(), view.Drill.ID)
	if unchanged.Drill.Revision != view.Drill.Revision || unchanged.Drill.Activation != nil {
		t.Fatalf("invalid activation changed aggregate: %+v", unchanged.Drill)
	}

	evidence := featureActivation(featureNow.Add(time.Hour))
	evidence.CommunicationGear.ValidUntil = featureNow.Add(20 * time.Minute).Format(time.RFC3339)
	registered, err := service.RegisterActivation(context.Background(), view.Drill.ID, ActivationInput{Mutation: Mutation{RequestID: "valid-activation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, ActivationEvidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return featureNow.Add(30 * time.Minute) }
	_, err = service.Start(context.Background(), view.Drill.ID, Mutation{RequestID: "expired-start", ExpectedRevision: registered.Drill.Revision, ActorID: "coord"})
	if !domain.IsValidation(err) || !strings.Contains(err.Error(), "communication_gear") {
		t.Fatalf("expected communication expiry field, got %v", err)
	}
	unchanged, _ = service.Get(context.Background(), view.Drill.ID)
	if unchanged.Drill.Revision != registered.Drill.Revision || unchanged.Drill.State != domain.StateFrozen {
		t.Fatalf("expired start changed state: %+v", unchanged.Drill)
	}

	replacement := featureActivation(featureNow.Add(90 * time.Minute))
	replacement.EvacuationRoute.CollectedAt = featureNow.Add(30 * time.Minute).Format(time.RFC3339)
	replacement.CommunicationGear.CollectedAt = replacement.EvacuationRoute.CollectedAt
	replacement.AccessibleFacility.CollectedAt = replacement.EvacuationRoute.CollectedAt
	replacement.PersonnelReady.CollectedAt = replacement.EvacuationRoute.CollectedAt
	replaced, err := service.RegisterActivation(context.Background(), view.Drill.ID, ActivationInput{Mutation: Mutation{RequestID: "replace-activation", ExpectedRevision: registered.Drill.Revision, ActorID: "coord"}, ActivationEvidence: replacement})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background(), view.Drill.ID, Mutation{RequestID: "start-after-replace", ExpectedRevision: replaced.Drill.Revision, ActorID: "coord"}); err != nil {
		t.Fatal(err)
	}
	events, _ := service.Timeline(context.Background(), view.Drill.ID)
	registeredEvents := 0
	for _, event := range events {
		if event.Type == "activation.recorded" {
			registeredEvents++
		}
	}
	if registeredEvents != 2 {
		t.Fatalf("expected two activation audit events, got %d", registeredEvents)
	}
}

func TestObservationReceiptAndRejectedSequenceAreAtomic(t *testing.T) {
	service := testService(t)
	service.now = func() time.Time { return featureNow }
	view := featureStart(t, service, []domain.Checkpoint{
		{Sequence: 1, Name: "集结", ResponsibleRole: "疏散岗", TimeLimitSeconds: 120, MinimumCapacity: 30, CommunicationRequired: true, AccessibilityRequired: true},
		{Sequence: 2, Name: "通信", ResponsibleRole: "通信岗", TimeLimitSeconds: 60, MinimumCapacity: 10, CommunicationRequired: true},
	})
	wrong := ObservationInput{Mutation: Mutation{RequestID: "wrong-sequence", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, CheckpointID: view.Drill.Checkpoints[1].ID, ObservedAt: featureNow.Format(time.RFC3339), ParticipantCount: 10, ElapsedSeconds: 50, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: featureDigest("wrong")}
	if _, err := service.SubmitObservation(context.Background(), view.Drill.ID, wrong); !domain.IsValidation(err) {
		t.Fatalf("expected sequence validation, got %v", err)
	}
	unchanged, _ := service.Get(context.Background(), view.Drill.ID)
	if unchanged.Drill.Revision != view.Drill.Revision || len(unchanged.Drill.Receipts) != 0 || unchanged.NextCheckpoint.ID != view.Drill.Checkpoints[0].ID {
		t.Fatalf("rejected observation changed progress: %+v", unchanged)
	}

	valid := wrong
	valid.RequestID = "receipt-observation"
	valid.CheckpointID = view.Drill.Checkpoints[0].ID
	valid.ParticipantCount, valid.ElapsedSeconds = 25, 150
	result, err := service.SubmitObservation(context.Background(), view.Drill.ID, valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Drill.Receipts) != 1 || result.LatestReceipt.ParticipantShortfall != 5 || result.LatestReceipt.OvertimeSeconds != 30 {
		t.Fatalf("unexpected receipt: %+v", result.LatestReceipt)
	}
	if len(result.Drill.Findings) != 2 || result.Drill.Findings[0].ReceiptID != result.LatestReceipt.ID || result.Drill.Findings[1].ReceiptID != result.LatestReceipt.ID {
		t.Fatalf("findings not linked to receipt: %+v", result.Drill.Findings)
	}
}

func TestRemediationPlanChangeAndOverdueRetest(t *testing.T) {
	service := testService(t)
	service.now = func() time.Time { return featureNow }
	view := featureStart(t, service, []domain.Checkpoint{{Sequence: 1, Name: "集结", ResponsibleRole: "疏散岗", TimeLimitSeconds: 120, MinimumCapacity: 30}})
	observed, err := service.SubmitObservation(context.Background(), view.Drill.ID, ObservationInput{Mutation: Mutation{RequestID: "failed-observation", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, CheckpointID: view.Drill.Checkpoints[0].ID, ObservedAt: featureNow.Format(time.RFC3339), ParticipantCount: 25, ElapsedSeconds: 100, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: featureDigest("live")})
	if err != nil {
		t.Fatal(err)
	}
	findingID := observed.Drill.Findings[0].ID
	planned, err := service.PlanRemediation(context.Background(), view.Drill.ID, findingID, RemediationInput{Mutation: Mutation{RequestID: "initial-plan", ExpectedRevision: observed.Drill.Revision, ActorID: "coord"}, Cause: "签到缺失", CorrectiveAction: "补齐签到", OwnerID: "owner-a", RetestPlannedAt: featureNow.Add(20 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := service.PlanRemediation(context.Background(), view.Drill.ID, findingID, RemediationInput{Mutation: Mutation{RequestID: "change-plan", ExpectedRevision: planned.Drill.Revision, ActorID: "coord"}, CorrectiveAction: "复核名册并补齐签到", OwnerID: "owner-b", RetestPlannedAt: featureNow.Add(30 * time.Minute).Format(time.RFC3339), ChangeReason: "责任人轮班"})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Drill.Findings[0].PlanHistory) != 2 || changed.Drill.Findings[0].OwnerID != "owner-b" {
		t.Fatalf("plan history not retained: %+v", changed.Drill.Findings[0])
	}
	_, err = service.PlanRemediation(context.Background(), view.Drill.ID, findingID, RemediationInput{Mutation: Mutation{RequestID: "late-plan", ExpectedRevision: changed.Drill.Revision, ActorID: "coord"}, CorrectiveAction: "无效计划", OwnerID: "owner-c", RetestPlannedAt: featureNow.Add(3 * time.Hour).Format(time.RFC3339), ChangeReason: "延后"})
	if !domain.IsValidation(err) {
		t.Fatalf("expected end boundary validation, got %v", err)
	}
	service.now = func() time.Time { return featureNow.Add(40 * time.Minute) }
	overdue, _ := service.Get(context.Background(), view.Drill.ID)
	if overdue.Todo[0].ResourceID != findingID || overdue.Todo[0].DueStatus != "overdue" || overdue.Todo[0].OverdueSeconds != 600 {
		t.Fatalf("unexpected overdue queue: %+v", overdue.Todo)
	}
	closed, err := service.SubmitRetest(context.Background(), view.Drill.ID, findingID, RetestInput{Mutation: Mutation{RequestID: "overdue-retest", ExpectedRevision: changed.Drill.Revision, ActorID: "coord"}, ObservedAt: featureNow.Add(40 * time.Minute).Format(time.RFC3339), ParticipantCount: 30, ElapsedSeconds: 100, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: featureDigest("retest")})
	if err != nil || closed.Drill.Findings[0].Status != domain.FindingClosed {
		t.Fatalf("overdue retest did not close finding: %v %+v", err, closed.Drill.Findings[0])
	}
}

func TestStructuredReviewChecklistSupportsDirectedReturnAndApproval(t *testing.T) {
	service := testService(t)
	service.now = func() time.Time { return featureNow }
	view := featureStart(t, service, []domain.Checkpoint{{Sequence: 1, Name: "应急通信", ResponsibleRole: "通信岗", TimeLimitSeconds: 120, MinimumCapacity: 30, CommunicationRequired: true}})
	observed, err := service.SubmitObservation(context.Background(), view.Drill.ID, ObservationInput{Mutation: Mutation{RequestID: "passed-live", ExpectedRevision: view.Drill.Revision, ActorID: "coord"}, CheckpointID: view.Drill.Checkpoints[0].ID, ObservedAt: featureNow.Format(time.RFC3339), ParticipantCount: 30, ElapsedSeconds: 100, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: featureDigest("review-live")})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := service.SubmitReview(context.Background(), view.Drill.ID, Mutation{RequestID: "submit-review", ExpectedRevision: observed.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	results := make([]domain.ReviewChecklistResult, 0, len(pending.Drill.CurrentSnapshot.Checklist))
	for _, item := range pending.Drill.CurrentSnapshot.Checklist {
		conclusion, opinion := "passed", "核验一致"
		if item.Kind == "judgment" {
			conclusion, opinion = "returned", "通信检查点现场判定需补充复核"
		}
		results = append(results, domain.ReviewChecklistResult{ItemID: item.ID, Conclusion: conclusion, Opinion: opinion})
	}
	_, err = service.ReviewDecision(context.Background(), view.Drill.ID, ReviewDecisionInput{Mutation: Mutation{RequestID: "coordinator-review", ExpectedRevision: pending.Drill.Revision, ActorID: "coord"}, Decision: "returned", Reason: "职责未分离", SnapshotDigest: pending.Drill.CurrentSnapshot.PayloadDigest, Checklist: results})
	if err != domain.ErrForbidden {
		t.Fatalf("expected role separation rejection, got %v", err)
	}
	returned, err := service.ReviewDecision(context.Background(), view.Drill.ID, ReviewDecisionInput{Mutation: Mutation{RequestID: "directed-return", ExpectedRevision: pending.Drill.Revision, ActorID: "reviewer"}, Decision: "returned", Reason: "定向补充证据", SnapshotDigest: pending.Drill.CurrentSnapshot.PayloadDigest, Checklist: results})
	if err != nil {
		t.Fatal(err)
	}
	returnedFinding := returned.Drill.Findings[len(returned.Drill.Findings)-1]
	if returnedFinding.CheckpointID != view.Drill.Checkpoints[0].ID || returnedFinding.ReviewChecklistItemID == "" {
		t.Fatalf("return finding not directed: %+v", returnedFinding)
	}
	planned, err := service.PlanRemediation(context.Background(), view.Drill.ID, returnedFinding.ID, RemediationInput{Mutation: Mutation{RequestID: "return-plan", ExpectedRevision: returned.Drill.Revision, ActorID: "coord"}, Cause: "复核证据不足", CorrectiveAction: "补充通信日志", OwnerID: "owner", RetestPlannedAt: featureNow.Add(10 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := service.SubmitRetest(context.Background(), view.Drill.ID, returnedFinding.ID, RetestInput{Mutation: Mutation{RequestID: "return-retest", ExpectedRevision: planned.Drill.Revision, ActorID: "coord"}, ObservedAt: featureNow.Add(5 * time.Minute).Format(time.RFC3339), ParticipantCount: 30, ElapsedSeconds: 100, CommunicationPassed: true, AccessibilityPassed: true, EvidenceDigest: featureDigest("return-retest")})
	if err != nil {
		t.Fatal(err)
	}
	pendingAgain, err := service.SubmitReview(context.Background(), view.Drill.ID, Mutation{RequestID: "submit-again", ExpectedRevision: closed.Drill.Revision, ActorID: "coord"})
	if err != nil {
		t.Fatal(err)
	}
	approvedResults := make([]domain.ReviewChecklistResult, 0, len(pendingAgain.Drill.CurrentSnapshot.Checklist))
	for _, item := range pendingAgain.Drill.CurrentSnapshot.Checklist {
		approvedResults = append(approvedResults, domain.ReviewChecklistResult{ItemID: item.ID, Conclusion: "passed", Opinion: "逐项核验通过"})
	}
	approved, err := service.ReviewDecision(context.Background(), view.Drill.ID, ReviewDecisionInput{Mutation: Mutation{RequestID: "approve", ExpectedRevision: pendingAgain.Drill.Revision, ActorID: "reviewer"}, Decision: "approved", Reason: "全部清单通过", SnapshotDigest: pendingAgain.Drill.CurrentSnapshot.PayloadDigest, Checklist: approvedResults})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Drill.State != domain.StateCertified || approved.Drill.Dossier.PassedChecklistCount != len(approvedResults) || !service.VerifyDossier(context.Background(), view.Drill.ID).Valid {
		t.Fatalf("dossier approval or verification failed: %+v", approved.Drill.Dossier)
	}
}
