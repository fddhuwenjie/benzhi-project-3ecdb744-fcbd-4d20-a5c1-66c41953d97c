package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"shelter-drill-gate/internal/domain"
)

type dossierPayload struct {
	DrillID              string                `json:"drill_id"`
	BaselineVersion      int                   `json:"baseline_version"`
	SnapshotID           string                `json:"snapshot_id"`
	Decision             string                `json:"decision"`
	ReviewerID           string                `json:"reviewer_id"`
	ReviewReason         string                `json:"review_reason"`
	ReviewedAt           string                `json:"reviewed_at"`
	Metrics              domain.DossierMetrics `json:"metrics"`
	RuleVersion          string                `json:"rule_version"`
	EventChainHead       string                `json:"event_chain_head"`
	ChecklistDigest      string                `json:"checklist_digest"`
	PassedChecklistCount int                   `json:"passed_checklist_count"`
}

func SnapshotDigest(snapshot domain.ReviewSnapshot) (string, error) {
	copy := snapshot
	copy.PayloadDigest = ""
	payload, err := CanonicalJSON(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func ChecklistDigest(results []domain.ReviewChecklistResult) (string, error) {
	ordered := append([]domain.ReviewChecklistResult(nil), results...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ItemID < ordered[j].ItemID })
	payload, err := CanonicalJSON(ordered)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func GenerateDossier(snapshot domain.ReviewSnapshot, reviewerID, reason, reviewedAt, eventChainHead, checklistDigest string, passedChecklistCount int, checklistResults []domain.ReviewChecklistResult) (domain.ReadinessDossier, error) {
	digest, err := SnapshotDigest(snapshot)
	if err != nil {
		return domain.ReadinessDossier{}, err
	}
	if snapshot.PayloadDigest != digest {
		return domain.ReadinessDossier{}, fmt.Errorf("送审快照摘要不匹配")
	}
	metrics := metricsFromSnapshot(snapshot)
	payload := dossierPayload{
		DrillID: snapshot.DrillID, BaselineVersion: snapshot.BaselineVersion,
		SnapshotID: snapshot.ID, Decision: "approved", ReviewerID: reviewerID,
		ReviewReason: reason, ReviewedAt: reviewedAt, Metrics: metrics,
		RuleVersion: snapshot.RuleVersion, EventChainHead: eventChainHead,
		ChecklistDigest: checklistDigest, PassedChecklistCount: passedChecklistCount,
	}
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return domain.ReadinessDossier{}, err
	}
	sum := sha256.Sum256(canonical)
	return domain.ReadinessDossier{
		DrillID: snapshot.DrillID, BaselineVersion: snapshot.BaselineVersion,
		SnapshotID: snapshot.ID, Decision: "approved", ReviewerID: reviewerID,
		ReviewReason: reason, ReviewedAt: reviewedAt, Metrics: metrics,
		RuleVersion: snapshot.RuleVersion, EventChainHead: eventChainHead,
		CanonicalPayload: string(canonical), DocumentDigest: hex.EncodeToString(sum[:]),
		ChecklistDigest: checklistDigest, PassedChecklistCount: passedChecklistCount,
		ChecklistResults: append([]domain.ReviewChecklistResult(nil), checklistResults...),
	}, nil
}

func VerifyDossier(dossier domain.ReadinessDossier, snapshot domain.ReviewSnapshot, events []Event) error {
	if dossier.Decision != "approved" || dossier.DrillID != snapshot.DrillID || dossier.SnapshotID != snapshot.ID {
		return fmt.Errorf("档案与送审快照的标识不一致")
	}
	if err := VerifyChain(events); err != nil {
		return err
	}
	foundHead := dossier.EventChainHead == "" && len(events) == 0
	for _, event := range events {
		if event.Hash == dossier.EventChainHead {
			foundHead = true
			break
		}
	}
	if !foundHead {
		return fmt.Errorf("档案引用的事件链头不存在")
	}
	checklistDigest, err := ChecklistDigest(dossier.ChecklistResults)
	if err != nil || checklistDigest != dossier.ChecklistDigest {
		return fmt.Errorf("复核清单摘要不匹配")
	}
	passed := 0
	expectedItems := make(map[string]bool, len(snapshot.Checklist))
	for _, item := range snapshot.Checklist {
		expectedItems[item.ID] = true
	}
	seenItems := make(map[string]bool, len(dossier.ChecklistResults))
	for _, result := range dossier.ChecklistResults {
		if result.Conclusion != "passed" {
			return fmt.Errorf("认证档案包含未通过的复核清单项")
		}
		if !expectedItems[result.ItemID] || seenItems[result.ItemID] {
			return fmt.Errorf("认证档案复核清单项目不匹配")
		}
		seenItems[result.ItemID] = true
		passed++
	}
	if passed != dossier.PassedChecklistCount || passed != len(snapshot.Checklist) {
		return fmt.Errorf("复核清单通过数量不匹配")
	}
	expected, err := GenerateDossier(snapshot, dossier.ReviewerID, dossier.ReviewReason, dossier.ReviewedAt, dossier.EventChainHead, dossier.ChecklistDigest, dossier.PassedChecklistCount, dossier.ChecklistResults)
	if err != nil {
		return err
	}
	if expected.CanonicalPayload != dossier.CanonicalPayload || expected.DocumentDigest != dossier.DocumentDigest {
		return fmt.Errorf("档案内容或 SHA-256 摘要发生漂移")
	}
	var payload dossierPayload
	if err := json.Unmarshal([]byte(dossier.CanonicalPayload), &payload); err != nil {
		return fmt.Errorf("档案规范载荷无效: %w", err)
	}
	return nil
}

func metricsFromSnapshot(snapshot domain.ReviewSnapshot) domain.DossierMetrics {
	metrics := domain.DossierMetrics{CheckpointCount: len(snapshot.Baseline), ObservationCount: len(snapshot.Observations), FindingCount: len(snapshot.Findings)}
	for _, finding := range snapshot.Findings {
		if finding.Status == domain.FindingClosed {
			metrics.ClosedFindingCount++
		}
	}
	for _, observation := range snapshot.Observations {
		if observation.ElapsedSeconds > metrics.MaxElapsedSeconds {
			metrics.MaxElapsedSeconds = observation.ElapsedSeconds
		}
		if observation.ObservationKind == "live" {
			metrics.TotalParticipants += observation.ParticipantCount
		}
	}
	return metrics
}
