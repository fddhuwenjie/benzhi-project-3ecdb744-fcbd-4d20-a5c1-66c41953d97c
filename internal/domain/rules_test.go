package domain

import "testing"

func TestEvaluateDeterministicFailures(t *testing.T) {
	checkpoint := Checkpoint{MinimumCapacity: 20, TimeLimitSeconds: 60, CommunicationRequired: true, AccessibilityRequired: true}
	observation := Observation{ParticipantCount: 10, ElapsedSeconds: 90, CommunicationPassed: false, AccessibilityPassed: false}
	result := Evaluate(checkpoint, observation)
	if result.Passed || len(result.Failures) != 4 {
		t.Fatalf("unexpected evaluation: %+v", result)
	}
	if result.Failures[0].Code != "CAPACITY_MIN" || result.Failures[0].Severity != SeverityCritical {
		t.Fatalf("unexpected first failure: %+v", result.Failures[0])
	}
}

func TestValidateDraftRequiresContinuousCheckpointSequence(t *testing.T) {
	drill := DrillCase{Title: "演练", SiteName: "场所", CoordinatorID: "c", ScenarioRisks: []string{"地震"}, PlannedStart: "2026-01-01T00:00:00Z", PlannedEnd: "2026-01-01T01:00:00Z", Checkpoints: []Checkpoint{{Sequence: 1, Name: "一", ResponsibleRole: "岗", TimeLimitSeconds: 1, MinimumCapacity: 1}, {Sequence: 3, Name: "三", ResponsibleRole: "岗", TimeLimitSeconds: 1, MinimumCapacity: 1}}}
	if err := ValidateDraft(drill); !IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
