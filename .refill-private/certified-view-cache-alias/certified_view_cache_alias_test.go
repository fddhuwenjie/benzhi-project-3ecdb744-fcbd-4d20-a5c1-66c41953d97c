package certified_view_cache_alias_test

import (
	"context"
	"testing"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/workflow"
)

func TestCertifiedViewCacheResultIsolation(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	const (
		drillID = "drill-cache-isolation"
		now     = "2026-08-27T10:00:00Z"
	)
	dossier := &domain.ReadinessDossier{
		DrillID: drillID, SnapshotID: "snapshot-cache-isolation", Decision: "approved",
		ReviewerID: "reviewer", ReviewedAt: now, DocumentDigest: "digest",
		ChecklistResults: []domain.ReviewChecklistResult{{
			ItemID: "baseline", Conclusion: "passed", Opinion: "初始复核意见",
		}},
	}
	drill := domain.DrillCase{
		ID: drillID, Title: "缓存隔离演练", State: domain.StateCertified, Revision: 1,
		ScenarioRisks: []string{"地震"}, CreatedAt: now, UpdatedAt: now,
		Checkpoints: []domain.Checkpoint{{ID: "checkpoint-1", DrillID: drillID, Sequence: 1, Name: "原始检查点"}},
		Receipts: []domain.JudgmentReceipt{{
			ID: "receipt-1", DrillID: drillID, CheckpointID: "checkpoint-1", ObservationID: "observation-1", CreatedAt: now,
			Items: []domain.JudgmentItem{{RuleCode: "TIME_LIMIT", Label: "原始判定"}},
		}},
		Findings: []domain.Finding{{
			ID: "finding-1", DrillID: drillID, CheckpointID: "checkpoint-1", RuleCode: "TIME_LIMIT",
			Status: domain.FindingClosed, Severity: domain.SeverityMajor,
			PlanHistory: []domain.RemediationPlanVersion{{Version: 1, CorrectiveAction: "原始措施"}},
		}},
		CurrentSnapshot: &domain.ReviewSnapshot{
			ID: "snapshot-cache-isolation", DrillID: drillID, CreatedAt: now, PayloadDigest: "snapshot-digest",
			Checklist: []domain.ReviewChecklistItem{{ID: "baseline", Label: "原始快照清单"}},
		},
		ReviewHistory: []domain.ReviewRecord{{
			Decision: "approved", Results: []domain.ReviewChecklistResult{{ItemID: "baseline", Conclusion: "passed", Opinion: "原始历史意见"}},
		}},
		Dossier: dossier,
	}
	_, err = repository.Commit(context.Background(), store.CommitInput{
		Drill: drill, RequestID: "seed-certified-drill", Fingerprint: "seed-fingerprint",
		ActorID: "reviewer", EventType: "review.approved", EventPayload: map[string]string{"snapshot_id": dossier.SnapshotID},
		OccurredAt: now, Response: []byte(`{"seeded":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	service := workflow.New(repository)
	first, err := service.Get(context.Background(), drillID)
	if err != nil {
		t.Fatal(err)
	}
	first.Drill.ScenarioRisks[0] = "调用方改写风险"
	first.Drill.Checkpoints[0].Name = "调用方改写检查点"
	first.Drill.Receipts[0].Items[0].Label = "调用方改写判定"
	first.Drill.Findings[0].PlanHistory[0].CorrectiveAction = "调用方改写措施"
	first.Drill.CurrentSnapshot.Checklist[0].Label = "调用方改写快照"
	first.Drill.ReviewHistory[0].Results[0].Opinion = "调用方改写历史"
	first.Drill.Dossier.ChecklistResults[0].Opinion = "调用方改写档案"

	second, err := service.Get(context.Background(), drillID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Drill.ScenarioRisks[0] != "地震" ||
		second.Drill.Checkpoints[0].Name != "原始检查点" ||
		second.Drill.Receipts[0].Items[0].Label != "原始判定" ||
		second.Drill.Findings[0].PlanHistory[0].CorrectiveAction != "原始措施" ||
		second.Drill.CurrentSnapshot.Checklist[0].Label != "原始快照清单" ||
		second.Drill.ReviewHistory[0].Results[0].Opinion != "原始历史意见" ||
		second.Drill.Dossier.ChecklistResults[0].Opinion != "初始复核意见" {
		t.Fatalf("certified view cache was polluted through nested aliases: %+v", second.Drill)
	}
}
