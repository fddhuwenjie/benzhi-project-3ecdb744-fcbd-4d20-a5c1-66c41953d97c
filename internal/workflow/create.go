package workflow

import (
	"context"
	"strings"

	"shelter-drill-gate/internal/domain"
)

type CreateInput struct {
	RequestID     string              `json:"request_id"`
	ActorID       string              `json:"actor_id"`
	Title         string              `json:"title"`
	SiteName      string              `json:"site_name"`
	ScenarioRisks []string            `json:"scenario_risks"`
	PlannedStart  string              `json:"planned_start"`
	PlannedEnd    string              `json:"planned_end"`
	Checkpoints   []domain.Checkpoint `json:"checkpoints"`
}

func (s *Service) Create(ctx context.Context, input CreateInput) (DrillView, error) {
	if strings.TrimSpace(input.RequestID) == "" {
		return DrillView{}, domain.Invalid("request_id", "不能为空")
	}
	if strings.TrimSpace(input.ActorID) == "" {
		return DrillView{}, domain.Invalid("actor_id", "不能为空")
	}
	fp, err := fingerprint("create", input)
	if err != nil {
		return DrillView{}, err
	}
	if result, found, err := s.replay(ctx, input.RequestID, fp); found || err != nil {
		return result, err
	}
	now := s.mutationTime()
	drill := domain.DrillCase{
		ID: newID("drill"), Title: strings.TrimSpace(input.Title), SiteName: strings.TrimSpace(input.SiteName),
		ScenarioRisks: input.ScenarioRisks, PlannedStart: input.PlannedStart, PlannedEnd: input.PlannedEnd,
		CoordinatorID: input.ActorID, State: domain.StateDraft, BaselineVersion: 0,
		RuleVersion: domain.CurrentRuleVersion, Revision: 1, CreatedAt: now, UpdatedAt: now,
		Checkpoints: append([]domain.Checkpoint(nil), input.Checkpoints...), Observations: []domain.Observation{}, Findings: []domain.Finding{},
	}
	for index := range drill.Checkpoints {
		drill.Checkpoints[index].ID = newID("cp")
		drill.Checkpoints[index].DrillID = drill.ID
	}
	sortCheckpoints(drill.Checkpoints)
	if err := domain.ValidateDraft(drill); err != nil {
		return DrillView{}, err
	}
	return s.commit(ctx, drill, nil, input.RequestID, fp, input.ActorID, "drill.created", map[string]any{"title": drill.Title, "site_name": drill.SiteName})
}
