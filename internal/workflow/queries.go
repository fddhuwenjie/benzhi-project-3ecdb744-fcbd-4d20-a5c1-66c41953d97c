package workflow

import (
	"context"

	"shelter-drill-gate/internal/audit"
	"shelter-drill-gate/internal/domain"
)

type VerificationResult struct {
	Valid            bool   `json:"valid"`
	DocumentDigest   string `json:"document_digest"`
	RecomputedDigest string `json:"recomputed_digest"`
	Message          string `json:"message"`
}

func (s *Service) Timeline(ctx context.Context, drillID string) ([]audit.Event, error) {
	if _, err := s.store.Load(ctx, drillID); err != nil {
		return nil, err
	}
	events, err := s.store.Timeline(ctx, drillID)
	if err != nil {
		return nil, err
	}
	if err := audit.VerifyChain(events); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Service) Dossier(ctx context.Context, drillID string) (domain.ReadinessDossier, error) {
	drill, err := s.store.Load(ctx, drillID)
	if err != nil {
		return domain.ReadinessDossier{}, err
	}
	if drill.Dossier == nil {
		return domain.ReadinessDossier{}, domain.ErrNotFound
	}
	return *drill.Dossier, nil
}

func (s *Service) VerifyDossier(ctx context.Context, drillID string) VerificationResult {
	drill, err := s.store.Load(ctx, drillID)
	if err != nil {
		return VerificationResult{Message: err.Error()}
	}
	if drill.Dossier == nil || drill.CurrentSnapshot == nil {
		return VerificationResult{Message: "演练尚未生成就绪档案"}
	}
	events, err := s.store.Timeline(ctx, drillID)
	if err != nil {
		return VerificationResult{DocumentDigest: drill.Dossier.DocumentDigest, Message: err.Error()}
	}
	result := VerificationResult{DocumentDigest: drill.Dossier.DocumentDigest}
	recomputed, generateErr := audit.GenerateDossier(*drill.CurrentSnapshot, drill.Dossier.ReviewerID, drill.Dossier.ReviewReason, drill.Dossier.ReviewedAt, drill.Dossier.EventChainHead, drill.Dossier.ChecklistDigest, drill.Dossier.PassedChecklistCount, drill.Dossier.ChecklistResults)
	if generateErr == nil {
		result.RecomputedDigest = recomputed.DocumentDigest
	}
	if err := audit.VerifyDossier(*drill.Dossier, *drill.CurrentSnapshot, events); err != nil {
		result.Message = err.Error()
		return result
	}
	result.Valid, result.Message = true, "档案内容、送审快照与审计摘要链验证通过"
	return result
}
