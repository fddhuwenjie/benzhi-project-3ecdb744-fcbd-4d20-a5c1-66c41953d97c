package dossierapprovalbinding

import (
	"testing"

	"shelter-drill-gate/internal/audit"
	"shelter-drill-gate/internal/domain"
)

func TestDossierRejectsStaleHistoricalChainHead(t *testing.T) {
	snapshot := domain.ReviewSnapshot{
		ID: "snapshot", DrillID: "drill", Revision: 4, BaselineVersion: 1,
		RuleVersion: "shelter-rules/1.0", CreatedAt: "2026-01-01T00:03:00Z",
		Checklist: []domain.ReviewChecklistItem{{ID: "audit_chain", Kind: "audit", Label: "审计链连续性"}},
	}
	digest, err := audit.SnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.PayloadDigest = digest
	results := []domain.ReviewChecklistResult{{ItemID: "audit_chain", Conclusion: "passed", Opinion: "核验通过"}}
	checklistDigest, err := audit.ChecklistDigest(results)
	if err != nil {
		t.Fatal(err)
	}
	created, err := audit.NewEvent("drill", 1, "drill.created", "coord", "2026-01-01T00:00:00Z", "", map[string]any{"title": "演练"})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := audit.NewEvent("drill", 2, "review.submitted", "coord", "2026-01-01T00:03:00Z", created.Hash, map[string]any{"snapshot_id": "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	dossier, err := audit.GenerateDossier(snapshot, "reviewer", "批准", "2026-01-01T00:04:00Z", created.Hash, checklistDigest, 1, results)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := audit.NewEvent("drill", 3, "review.approved", "reviewer", "2026-01-01T00:04:00Z", submitted.Hash, map[string]any{"document_digest": dossier.DocumentDigest})
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.VerifyDossier(dossier, snapshot, []audit.Event{created, submitted, approved}); err == nil {
		t.Fatal("TestDossierRejectsStaleHistoricalChainHead: verifier accepted a dossier bound to an old event instead of the approval event predecessor")
	}
}
