package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"shelter-drill-gate/internal/audit"
	"shelter-drill-gate/internal/domain"
)

func (s *Service) SubmitReview(ctx context.Context, drillID string, mutation Mutation) (DrillView, error) {
	if err := validateMutation(mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("submit-review:"+drillID, mutation)
	if err != nil {
		return DrillView{}, err
	}
	if result, found, err := s.replay(ctx, mutation.RequestID, fp); found || err != nil {
		return result, err
	}
	drill, err := s.store.Load(ctx, drillID)
	if err != nil {
		return DrillView{}, err
	}
	if err := ensureRevision(drill, mutation.ExpectedRevision); err != nil {
		return DrillView{}, err
	}
	if drill.CoordinatorID != mutation.ActorID {
		return DrillView{}, domain.ErrForbidden
	}
	if drill.State != domain.StateActive && drill.State != domain.StateRemediation {
		return DrillView{}, domain.Invalid("state", "当前状态不可送审")
	}
	if !drill.AllLiveObservationsSubmitted() {
		return DrillView{}, domain.Invalid("observations", "全部检查点完成现场记录后才能送审")
	}
	if drill.HasOpenCriticalFinding() {
		return DrillView{}, domain.Invalid("findings", "存在未关闭严重问题，禁止送审")
	}
	if len(drill.OpenFindings()) > 0 {
		return DrillView{}, domain.Invalid("findings", "所有整改项关闭后才能送审")
	}
	if err := drill.Transition(domain.StateReview); err != nil {
		return DrillView{}, err
	}
	snapshot, err := makeSnapshot(drill, mutation.ExpectedRevision+1, s.mutationTime())
	if err != nil {
		return DrillView{}, err
	}
	drill.CurrentSnapshot = &snapshot
	return s.advance(ctx, drill, mutation, fp, "review.submitted", map[string]any{"snapshot_id": snapshot.ID, "snapshot_digest": snapshot.PayloadDigest})
}

type ReviewDecisionInput struct {
	Mutation
	Decision       string                         `json:"decision"`
	Reason         string                         `json:"reason"`
	SnapshotDigest string                         `json:"snapshot_digest,omitempty"`
	Checklist      []domain.ReviewChecklistResult `json:"checklist"`
}

func (s *Service) ReviewDecision(ctx context.Context, drillID string, input ReviewDecisionInput) (DrillView, error) {
	if err := validateMutation(input.Mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("review-decision:"+drillID, input)
	if err != nil {
		return DrillView{}, err
	}
	if result, found, err := s.replay(ctx, input.RequestID, fp); found || err != nil {
		return result, err
	}
	drill, err := s.store.Load(ctx, drillID)
	if err != nil {
		return DrillView{}, err
	}
	if err := ensureRevision(drill, input.ExpectedRevision); err != nil {
		return DrillView{}, err
	}
	if drill.State != domain.StateReview || drill.CurrentSnapshot == nil {
		return DrillView{}, domain.Invalid("state", "仅待复核状态可作出决定")
	}
	if err := domain.ValidateReviewSeparation(drill.CoordinatorID, input.ActorID); err != nil {
		return DrillView{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return DrillView{}, domain.Invalid("reason", "复核理由不能为空")
	}
	if input.Decision != "returned" && input.Decision != "approved" {
		return DrillView{}, domain.Invalid("decision", "必须是 returned 或 approved")
	}
	recomputedSnapshotDigest, err := audit.SnapshotDigest(*drill.CurrentSnapshot)
	if err != nil {
		return DrillView{}, err
	}
	if recomputedSnapshotDigest != drill.CurrentSnapshot.PayloadDigest || (input.SnapshotDigest != "" && input.SnapshotDigest != recomputedSnapshotDigest) {
		return DrillView{}, domain.Invalid("snapshot_digest", "送审快照摘要复算不一致")
	}
	passedCount, returnedItems, err := validateChecklist(drill.CurrentSnapshot.Checklist, input.Checklist)
	if err != nil {
		return DrillView{}, err
	}
	checklistDigest, err := audit.ChecklistDigest(input.Checklist)
	if err != nil {
		return DrillView{}, err
	}
	decidedAt := s.mutationTime()
	record := domain.ReviewRecord{Decision: input.Decision, ReviewerID: input.ActorID, Reason: strings.TrimSpace(input.Reason), Results: input.Checklist, ChecklistDigest: checklistDigest, PassedCount: passedCount, DecidedAt: decidedAt}
	switch input.Decision {
	case "returned":
		if len(returnedItems) == 0 {
			return DrillView{}, domain.Invalid("checklist", "退回决定至少需要一项标记为 returned")
		}
		if err := drill.Transition(domain.StateRemediation); err != nil {
			return DrillView{}, err
		}
		for _, returned := range returnedItems {
			finding := domain.Finding{
				ID: newID("finding"), DrillID: drill.ID, CheckpointID: returned.CheckpointID,
				RuleCode: "REVIEW_RETURN", RuleVersion: drill.RuleVersion, Severity: domain.SeverityMajor,
				Status: domain.FindingOpen, Cause: opinionFor(input.Checklist, returned.ID), OpenedAt: decidedAt,
				ReviewChecklistItemID: returned.ID, EvidenceCategory: returned.EvidenceCategory,
			}
			if returned.EvidenceCategory != "" && returned.CheckpointID == "" {
				finding.OriginalEvidenceDigest = snapshotActivationDigest(*drill.CurrentSnapshot, returned.EvidenceCategory)
			}
			drill.Findings = append(drill.Findings, finding)
		}
		drill.ReviewHistory = append(drill.ReviewHistory, record)
		drill.CurrentSnapshot = nil
		return s.advance(ctx, drill, input.Mutation, fp, "review.returned", map[string]any{"reason": record.Reason, "checklist": input.Checklist, "checklist_digest": checklistDigest})
	case "approved":
		if len(returnedItems) > 0 || passedCount != len(drill.CurrentSnapshot.Checklist) {
			return DrillView{}, domain.Invalid("checklist", "批准要求所有清单项均为 passed")
		}
		events, err := s.store.Timeline(ctx, drill.ID)
		if err != nil {
			return DrillView{}, err
		}
		if err := audit.VerifyChain(events); err != nil {
			return DrillView{}, err
		}
		head := ""
		if len(events) > 0 {
			head = events[len(events)-1].Hash
		}
		dossier, err := audit.GenerateDossier(*drill.CurrentSnapshot, input.ActorID, record.Reason, decidedAt, head, checklistDigest, passedCount, input.Checklist)
		if err != nil {
			return DrillView{}, err
		}
		if err := drill.Transition(domain.StateCertified); err != nil {
			return DrillView{}, err
		}
		drill.Dossier = &dossier
		drill.ReviewHistory = append(drill.ReviewHistory, record)
		return s.advance(ctx, drill, input.Mutation, fp, "review.approved", map[string]any{"snapshot_id": dossier.SnapshotID, "document_digest": dossier.DocumentDigest, "checklist": input.Checklist, "checklist_digest": checklistDigest, "passed_count": passedCount})
	}
	return DrillView{}, domain.Invalid("decision", "必须是 returned 或 approved")
}

func makeSnapshot(drill domain.DrillCase, revision int64, createdAt string) (domain.ReviewSnapshot, error) {
	snapshot := domain.ReviewSnapshot{
		ID: newID("snapshot"), DrillID: drill.ID, Revision: revision,
		BaselineVersion: drill.BaselineVersion, RuleVersion: drill.RuleVersion,
		CreatedAt: createdAt, Activation: *drill.Activation,
	}
	if err := deepCopy(drill.Checkpoints, &snapshot.Baseline); err != nil {
		return snapshot, err
	}
	if err := deepCopy(drill.Observations, &snapshot.Observations); err != nil {
		return snapshot, err
	}
	if err := deepCopy(drill.Findings, &snapshot.Findings); err != nil {
		return snapshot, err
	}
	if err := deepCopy(drill.Receipts, &snapshot.Receipts); err != nil {
		return snapshot, err
	}
	snapshot.Checklist = buildReviewChecklist(snapshot)
	digest, err := audit.SnapshotDigest(snapshot)
	if err != nil {
		return snapshot, err
	}
	snapshot.PayloadDigest = digest
	return snapshot, nil
}

func buildReviewChecklist(snapshot domain.ReviewSnapshot) []domain.ReviewChecklistItem {
	items := []domain.ReviewChecklistItem{{ID: "baseline", Kind: "baseline", Label: "基线完整性"}}
	labels := map[string]string{"evacuation_route": "疏散路线启用证据", "communication_gear": "通信设备启用证据", "accessible_facility": "无障碍设施启用证据", "personnel_ready": "人员到位启用证据"}
	for _, category := range []string{"evacuation_route", "communication_gear", "accessible_facility", "personnel_ready"} {
		items = append(items, domain.ReviewChecklistItem{ID: "activation:" + category, Kind: "activation", Label: labels[category], EvidenceCategory: category})
	}
	for _, receipt := range snapshot.Receipts {
		items = append(items, domain.ReviewChecklistItem{ID: "judgment:" + receipt.ID, Kind: "judgment", Label: fmt.Sprintf("检查点 %d 现场判定", receipt.CheckpointSequence), CheckpointID: receipt.CheckpointID})
	}
	for _, finding := range snapshot.Findings {
		items = append(items, domain.ReviewChecklistItem{ID: "finding:" + finding.ID, Kind: "remediation", Label: "整改闭环 " + finding.RuleCode, CheckpointID: finding.CheckpointID, EvidenceCategory: finding.EvidenceCategory})
	}
	items = append(items, domain.ReviewChecklistItem{ID: "audit_chain", Kind: "audit", Label: "审计链连续性"})
	return items
}

func validateChecklist(expected []domain.ReviewChecklistItem, actual []domain.ReviewChecklistResult) (int, []domain.ReviewChecklistItem, error) {
	lookup := make(map[string]domain.ReviewChecklistItem, len(expected))
	for _, item := range expected {
		lookup[item.ID] = item
	}
	seen := make(map[string]bool, len(actual))
	passed := 0
	returned := make([]domain.ReviewChecklistItem, 0)
	fields := make([]domain.FieldError, 0)
	for index, result := range actual {
		prefix := fmt.Sprintf("checklist[%d]", index)
		item, exists := lookup[result.ItemID]
		if !exists {
			fields = append(fields, domain.FieldError{Field: prefix + ".item_id", Message: "未知清单项"})
			continue
		}
		if seen[result.ItemID] {
			fields = append(fields, domain.FieldError{Field: prefix + ".item_id", Message: "清单项不得重复"})
			continue
		}
		seen[result.ItemID] = true
		if strings.TrimSpace(result.Opinion) == "" {
			fields = append(fields, domain.FieldError{Field: prefix + ".opinion", Message: "复核意见不能为空"})
		}
		switch result.Conclusion {
		case "passed":
			passed++
		case "returned":
			returned = append(returned, item)
		default:
			fields = append(fields, domain.FieldError{Field: prefix + ".conclusion", Message: "必须是 passed 或 returned"})
		}
	}
	for _, item := range expected {
		if !seen[item.ID] {
			fields = append(fields, domain.FieldError{Field: "checklist", Message: "缺少清单项 " + item.ID})
		}
	}
	if len(fields) > 0 {
		return 0, nil, &domain.ValidationError{Fields: fields}
	}
	return passed, returned, nil
}

func opinionFor(results []domain.ReviewChecklistResult, itemID string) string {
	for _, result := range results {
		if result.ItemID == itemID {
			return strings.TrimSpace(result.Opinion)
		}
	}
	return ""
}

func snapshotActivationDigest(snapshot domain.ReviewSnapshot, category string) string {
	for _, record := range domain.ActivationRecords(snapshot.Activation) {
		if record.Category == category {
			return record.Record.ContentDigest
		}
	}
	return ""
}

func deepCopy(source, target any) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
