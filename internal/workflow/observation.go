package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"shelter-drill-gate/internal/domain"
)

type ObservationInput struct {
	Mutation
	CheckpointID        string `json:"checkpoint_id"`
	ObservedAt          string `json:"observed_at"`
	ParticipantCount    int    `json:"participant_count"`
	ElapsedSeconds      int    `json:"elapsed_seconds"`
	CommunicationPassed bool   `json:"communication_passed"`
	AccessibilityPassed bool   `json:"accessibility_passed"`
	EvidenceDigest      string `json:"evidence_digest"`
}

func (s *Service) SubmitObservation(ctx context.Context, drillID string, input ObservationInput) (DrillView, error) {
	if err := validateMutation(input.Mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("observation:"+drillID, input)
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
	if drill.State != domain.StateActive {
		return DrillView{}, domain.Invalid("state", "仅进行中的演练可提交现场记录")
	}
	if drill.CoordinatorID != input.ActorID {
		return DrillView{}, domain.ErrForbidden
	}
	next, ok := drill.NextCheckpoint()
	if !ok {
		return DrillView{}, domain.Invalid("checkpoint_id", "全部现场检查点均已记录")
	}
	if next.ID != input.CheckpointID {
		return DrillView{}, domain.Invalid("checkpoint_id", "必须按冻结基线顺序提交，下一检查点为 "+next.ID)
	}
	observation := domain.Observation{
		ID: newID("obs"), DrillID: drill.ID, CheckpointID: next.ID, ObservationKind: "live",
		ObservedAt: input.ObservedAt, ObserverID: input.ActorID, ParticipantCount: input.ParticipantCount,
		ElapsedSeconds: input.ElapsedSeconds, CommunicationPassed: input.CommunicationPassed,
		AccessibilityPassed: input.AccessibilityPassed, EvidenceDigest: input.EvidenceDigest,
		SubmittedAt: s.mutationTime(),
	}
	if err := domain.ValidateObservationWindow(drill, observation); err != nil {
		return DrillView{}, err
	}
	evaluation := domain.Evaluate(next, observation)
	receipt := domain.BuildJudgmentReceipt(newID("receipt"), next, observation, drill.RuleVersion, observation.SubmittedAt)
	drill.Observations = append(drill.Observations, observation)
	drill.Receipts = append(drill.Receipts, receipt)
	for _, failure := range evaluation.Failures {
		drill.Findings = append(drill.Findings, domain.Finding{
			ID: newID("finding"), DrillID: drill.ID, CheckpointID: next.ID,
			RuleCode: failure.Code, RuleVersion: drill.RuleVersion, Severity: failure.Severity,
			Status: domain.FindingOpen, OpenedAt: observation.SubmittedAt, ReceiptID: receipt.ID,
		})
	}
	if drill.AllLiveObservationsSubmitted() && len(drill.OpenFindings()) > 0 {
		if err := drill.Transition(domain.StateRemediation); err != nil {
			return DrillView{}, err
		}
	}
	payload := map[string]any{"observation_id": observation.ID, "checkpoint_id": next.ID, "receipt_id": receipt.ID, "judgments": receipt.Items}
	return s.advance(ctx, drill, input.Mutation, fp, "observation.submitted", payload)
}

type RemediationInput struct {
	Mutation
	Cause            string `json:"cause"`
	CorrectiveAction string `json:"corrective_action"`
	OwnerID          string `json:"owner_id"`
	RetestPlannedAt  string `json:"retest_planned_at"`
	ChangeReason     string `json:"change_reason,omitempty"`
}

func (s *Service) PlanRemediation(ctx context.Context, drillID, findingID string, input RemediationInput) (DrillView, error) {
	if err := validateMutation(input.Mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("remediation:"+drillID+":"+findingID, input)
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
	if drill.State != domain.StateRemediation {
		return DrillView{}, domain.Invalid("state", "仅整改复测状态可登记措施")
	}
	if drill.CoordinatorID != input.ActorID {
		return DrillView{}, domain.ErrForbidden
	}
	index, finding, ok := drill.FindingByID(findingID)
	if !ok {
		return DrillView{}, domain.ErrNotFound
	}
	if finding.Status == domain.FindingClosed {
		return DrillView{}, domain.Invalid("finding.status", "已关闭问题禁止变更计划")
	}
	isChange := finding.Status == domain.FindingPlanned
	if !isChange && strings.TrimSpace(input.Cause) == "" {
		return DrillView{}, domain.Invalid("cause", "原因不能为空")
	}
	if isChange && strings.TrimSpace(input.ChangeReason) == "" {
		return DrillView{}, domain.Invalid("change_reason", "变更理由不能为空")
	}
	businessNow := s.now().UTC()
	if err := domain.ValidateRemediationPlan(input.OwnerID, input.CorrectiveAction, input.RetestPlannedAt, drill.PlannedEnd, businessNow); err != nil {
		return DrillView{}, err
	}
	before := map[string]string{"owner_id": finding.OwnerID, "corrective_action": finding.CorrectiveAction, "retest_planned_at": finding.RetestPlannedAt}
	if !isChange {
		finding.Cause = strings.TrimSpace(input.Cause)
	}
	finding.CorrectiveAction, finding.OwnerID = strings.TrimSpace(input.CorrectiveAction), strings.TrimSpace(input.OwnerID)
	finding.RetestPlannedAt, finding.Status = input.RetestPlannedAt, domain.FindingPlanned
	version := domain.RemediationPlanVersion{
		Version: len(finding.PlanHistory) + 1, Cause: finding.Cause, CorrectiveAction: finding.CorrectiveAction,
		OwnerID: finding.OwnerID, RetestPlannedAt: finding.RetestPlannedAt, ChangeReason: strings.TrimSpace(input.ChangeReason),
		ChangedBy: input.ActorID, ChangedAt: domain.NormalizeTime(businessNow),
	}
	finding.PlanHistory = append(finding.PlanHistory, version)
	drill.Findings[index] = finding
	eventType := "finding.remediation_planned"
	if isChange {
		eventType = "finding.plan_changed"
	}
	after := map[string]string{"owner_id": finding.OwnerID, "corrective_action": finding.CorrectiveAction, "retest_planned_at": finding.RetestPlannedAt}
	return s.advance(ctx, drill, input.Mutation, fp, eventType, map[string]any{"finding_id": finding.ID, "plan_version": version.Version, "change_reason": version.ChangeReason, "before": before, "after": after})
}

type RetestInput struct {
	Mutation
	ObservedAt          string `json:"observed_at"`
	ParticipantCount    int    `json:"participant_count"`
	ElapsedSeconds      int    `json:"elapsed_seconds"`
	CommunicationPassed bool   `json:"communication_passed"`
	AccessibilityPassed bool   `json:"accessibility_passed"`
	EvidenceDigest      string `json:"evidence_digest"`
}

func (s *Service) SubmitRetest(ctx context.Context, drillID, findingID string, input RetestInput) (DrillView, error) {
	if err := validateMutation(input.Mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("retest:"+drillID+":"+findingID, input)
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
	if drill.State != domain.StateRemediation {
		return DrillView{}, domain.Invalid("state", "仅整改复测状态可提交复测")
	}
	if drill.CoordinatorID != input.ActorID {
		return DrillView{}, domain.ErrForbidden
	}
	index, finding, ok := drill.FindingByID(findingID)
	if !ok {
		return DrillView{}, domain.ErrNotFound
	}
	if finding.Status != domain.FindingPlanned {
		return DrillView{}, domain.Invalid("finding.status", "必须先完整登记整改措施")
	}
	checkpoint, checkpointFound := drill.CheckpointByID(finding.CheckpointID)
	if !checkpointFound && finding.RuleCode != "REVIEW_RETURN" {
		return DrillView{}, fmt.Errorf("整改项引用了不存在的检查点")
	}
	observation := domain.Observation{
		ID: newID("retest"), DrillID: drill.ID, CheckpointID: checkpoint.ID, ObservationKind: "retest",
		ObservedAt: input.ObservedAt, ObserverID: input.ActorID, ParticipantCount: input.ParticipantCount,
		ElapsedSeconds: input.ElapsedSeconds, CommunicationPassed: input.CommunicationPassed,
		AccessibilityPassed: input.AccessibilityPassed, EvidenceDigest: input.EvidenceDigest,
		SubmittedAt: s.mutationTime(),
	}
	if err := domain.ValidateObservationWindow(drill, observation); err != nil {
		return DrillView{}, err
	}
	if finding.RuleCode != "REVIEW_RETURN" {
		if failure, failed := domain.FailureForRule(checkpoint, observation, finding.RuleCode); failed {
			return DrillView{}, domain.Invalid("retest", "复测仍未满足原规则："+failure.Message)
		}
	}
	drill.Observations = append(drill.Observations, observation)
	finding.Status, finding.RetestObservationID, finding.ClosedAt = domain.FindingClosed, observation.ID, observation.SubmittedAt
	drill.Findings[index] = finding
	overdueSeconds := int64(0)
	if due, parseErr := domain.ParseTime(finding.RetestPlannedAt); parseErr == nil && s.now().UTC().After(due) {
		overdueSeconds = int64(s.now().UTC().Sub(due) / time.Second)
	}
	return s.advance(ctx, drill, input.Mutation, fp, "finding.retest_passed", map[string]any{"finding_id": finding.ID, "observation_id": observation.ID, "rule_code": finding.RuleCode, "overdue_seconds": overdueSeconds})
}
