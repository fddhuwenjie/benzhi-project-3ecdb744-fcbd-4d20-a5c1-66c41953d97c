package workflow

import (
	"context"

	"shelter-drill-gate/internal/domain"
)

func (s *Service) Freeze(ctx context.Context, drillID string, mutation Mutation) (DrillView, error) {
	if err := validateMutation(mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("freeze:"+drillID, mutation)
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
	if err := domain.ValidateDraft(drill); err != nil {
		return DrillView{}, err
	}
	if err := drill.Transition(domain.StateFrozen); err != nil {
		return DrillView{}, err
	}
	drill.BaselineVersion++
	for index := range drill.Checkpoints {
		drill.Checkpoints[index].BaselineVersion = drill.BaselineVersion
	}
	return s.advance(ctx, drill, mutation, fp, "baseline.frozen", map[string]any{"baseline_version": drill.BaselineVersion, "rule_version": drill.RuleVersion})
}

type ActivationInput struct {
	Mutation
	domain.ActivationEvidence
}

func (s *Service) RegisterActivation(ctx context.Context, drillID string, input ActivationInput) (DrillView, error) {
	if err := validateMutation(input.Mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("activation:"+drillID, input)
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
	if drill.State != domain.StateFrozen && drill.State != domain.StateRemediation {
		return DrillView{}, domain.Invalid("state", "仅基线已冻结或整改复测状态可登记启用证据")
	}
	if drill.CoordinatorID != input.ActorID {
		return DrillView{}, domain.ErrForbidden
	}
	evidence := input.ActivationEvidence
	evidence.RecordedBy = input.ActorID
	evidence.RecordedAt = s.mutationTime()
	if err := domain.ValidateActivation(evidence, drill.PlannedStart); err != nil {
		return DrillView{}, err
	}
	replaced := drill.Activation != nil
	drill.Activation = &evidence
	items := make([]map[string]string, 0, 4)
	for _, item := range domain.ActivationRecords(evidence) {
		items = append(items, map[string]string{"category": item.Category, "collected_at": item.Record.CollectedAt, "valid_until": item.Record.ValidUntil, "content_digest": item.Record.ContentDigest})
	}
	return s.advance(ctx, drill, input.Mutation, fp, "activation.recorded", map[string]any{"evidence": items, "replaced": replaced})
}

func (s *Service) Start(ctx context.Context, drillID string, mutation Mutation) (DrillView, error) {
	if err := validateMutation(mutation); err != nil {
		return DrillView{}, err
	}
	fp, err := fingerprint("start:"+drillID, mutation)
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
	if drill.Activation == nil {
		fields := make([]domain.FieldError, 0, 4)
		for _, category := range []string{"evacuation_route", "communication_gear", "accessible_facility", "personnel_ready"} {
			fields = append(fields, domain.FieldError{Field: category, Message: "证据缺失，请重新登记"})
		}
		return DrillView{}, &domain.ValidationError{Fields: fields}
	}
	if err := domain.ValidateActivation(*drill.Activation, drill.PlannedStart); err != nil {
		return DrillView{}, err
	}
	statuses := activationStatuses(drill.Activation, drill.PlannedStart, s.now())
	fields := make([]domain.FieldError, 0)
	for _, status := range statuses {
		if !status.Valid {
			fields = append(fields, domain.FieldError{Field: status.Category, Message: status.Reason + "，请重新登记"})
		}
	}
	if len(fields) > 0 {
		return DrillView{}, &domain.ValidationError{Fields: fields}
	}
	if err := drill.Transition(domain.StateActive); err != nil {
		return DrillView{}, err
	}
	return s.advance(ctx, drill, mutation, fp, "drill.started", map[string]any{"baseline_version": drill.BaselineVersion})
}

func (s *Service) advance(ctx context.Context, drill domain.DrillCase, mutation Mutation, fp, eventType string, payload any) (DrillView, error) {
	expected := mutation.ExpectedRevision
	drill.Revision = expected + 1
	drill.UpdatedAt = s.mutationTime()
	return s.commit(ctx, drill, &expected, mutation.RequestID, fp, mutation.ActorID, eventType, payload)
}
