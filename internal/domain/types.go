package domain

import "time"

type DrillState string

const (
	StateDraft       DrillState = "draft"
	StateFrozen      DrillState = "baseline_frozen"
	StateActive      DrillState = "in_progress"
	StateRemediation DrillState = "remediation"
	StateReview      DrillState = "pending_review"
	StateCertified   DrillState = "certified"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
)

type FindingStatus string

const (
	FindingOpen    FindingStatus = "open"
	FindingPlanned FindingStatus = "planned"
	FindingClosed  FindingStatus = "closed"
)

type Checkpoint struct {
	ID                    string `json:"id"`
	DrillID               string `json:"drill_id"`
	Sequence              int    `json:"sequence"`
	Name                  string `json:"name"`
	ResponsibleRole       string `json:"responsible_role"`
	TimeLimitSeconds      int    `json:"time_limit_seconds"`
	MinimumCapacity       int    `json:"minimum_capacity"`
	CommunicationRequired bool   `json:"communication_required"`
	AccessibilityRequired bool   `json:"accessibility_required"`
	BaselineVersion       int    `json:"baseline_version"`
}

type EvidenceRecord struct {
	CollectedAt   string `json:"collected_at"`
	ValidUntil    string `json:"valid_until"`
	ContentDigest string `json:"content_digest"`
}

type ActivationEvidence struct {
	EvacuationRoute    EvidenceRecord `json:"evacuation_route"`
	CommunicationGear  EvidenceRecord `json:"communication_gear"`
	AccessibleFacility EvidenceRecord `json:"accessible_facility"`
	PersonnelReady     EvidenceRecord `json:"personnel_ready"`
	RecordedBy         string         `json:"recorded_by"`
	RecordedAt         string         `json:"recorded_at"`
}

type Observation struct {
	ID                  string `json:"id"`
	DrillID             string `json:"drill_id"`
	CheckpointID        string `json:"checkpoint_id"`
	ObservationKind     string `json:"observation_kind"`
	ObservedAt          string `json:"observed_at"`
	ObserverID          string `json:"observer_id"`
	ParticipantCount    int    `json:"participant_count"`
	ElapsedSeconds      int    `json:"elapsed_seconds"`
	CommunicationPassed bool   `json:"communication_passed"`
	AccessibilityPassed bool   `json:"accessibility_passed"`
	EvidenceDigest      string `json:"evidence_digest"`
	SubmittedAt         string `json:"submitted_at"`
}

type JudgmentItem struct {
	RuleCode       string   `json:"rule_code"`
	RuleVersion    string   `json:"rule_version"`
	Label          string   `json:"label"`
	ObservedValue  string   `json:"observed_value"`
	ThresholdValue string   `json:"threshold_value"`
	Passed         bool     `json:"passed"`
	Deviation      int      `json:"deviation"`
	Unit           string   `json:"unit,omitempty"`
	Severity       Severity `json:"severity,omitempty"`
}

type JudgmentReceipt struct {
	ID                   string         `json:"id"`
	DrillID              string         `json:"drill_id"`
	CheckpointID         string         `json:"checkpoint_id"`
	ObservationID        string         `json:"observation_id"`
	CheckpointSequence   int            `json:"checkpoint_sequence"`
	RuleVersion          string         `json:"rule_version"`
	ParticipantShortfall int            `json:"participant_shortfall"`
	OvertimeSeconds      int            `json:"overtime_seconds"`
	Items                []JudgmentItem `json:"items"`
	CreatedAt            string         `json:"created_at"`
}

type RemediationPlanVersion struct {
	Version          int    `json:"version"`
	Cause            string `json:"cause"`
	CorrectiveAction string `json:"corrective_action"`
	OwnerID          string `json:"owner_id"`
	RetestPlannedAt  string `json:"retest_planned_at"`
	ChangeReason     string `json:"change_reason"`
	ChangedBy        string `json:"changed_by"`
	ChangedAt        string `json:"changed_at"`
}

type Finding struct {
	ID                    string                   `json:"id"`
	DrillID               string                   `json:"drill_id"`
	CheckpointID          string                   `json:"checkpoint_id"`
	RuleCode              string                   `json:"rule_code"`
	RuleVersion           string                   `json:"rule_version"`
	Severity              Severity                 `json:"severity"`
	Status                FindingStatus            `json:"status"`
	Cause                 string                   `json:"cause,omitempty"`
	CorrectiveAction      string                   `json:"corrective_action,omitempty"`
	OwnerID               string                   `json:"owner_id,omitempty"`
	RetestPlannedAt       string                   `json:"retest_planned_at,omitempty"`
	RetestObservationID   string                   `json:"retest_observation_id,omitempty"`
	OpenedAt              string                   `json:"opened_at"`
	ClosedAt              string                   `json:"closed_at,omitempty"`
	ReceiptID             string                   `json:"receipt_id,omitempty"`
	ReviewChecklistItemID string                   `json:"review_checklist_item_id,omitempty"`
	EvidenceCategory      string                   `json:"evidence_category,omitempty"`
	PlanHistory           []RemediationPlanVersion `json:"plan_history,omitempty"`
}

type ReviewChecklistItem struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	CheckpointID     string `json:"checkpoint_id,omitempty"`
	EvidenceCategory string `json:"evidence_category,omitempty"`
}

type ReviewChecklistResult struct {
	ItemID     string `json:"item_id"`
	Conclusion string `json:"conclusion"`
	Opinion    string `json:"opinion"`
}

type ReviewRecord struct {
	Decision        string                  `json:"decision"`
	ReviewerID      string                  `json:"reviewer_id"`
	Reason          string                  `json:"reason"`
	Results         []ReviewChecklistResult `json:"results"`
	ChecklistDigest string                  `json:"checklist_digest"`
	PassedCount     int                     `json:"passed_count"`
	DecidedAt       string                  `json:"decided_at"`
}

type ReviewSnapshot struct {
	ID              string                `json:"id"`
	DrillID         string                `json:"drill_id"`
	Revision        int64                 `json:"revision"`
	BaselineVersion int                   `json:"baseline_version"`
	RuleVersion     string                `json:"rule_version"`
	CreatedAt       string                `json:"created_at"`
	Baseline        []Checkpoint          `json:"baseline"`
	Observations    []Observation         `json:"observations"`
	Findings        []Finding             `json:"findings"`
	Receipts        []JudgmentReceipt     `json:"receipts"`
	Checklist       []ReviewChecklistItem `json:"checklist"`
	Activation      ActivationEvidence    `json:"activation"`
	PayloadDigest   string                `json:"payload_digest"`
}

type DossierMetrics struct {
	CheckpointCount    int `json:"checkpoint_count"`
	ObservationCount   int `json:"observation_count"`
	FindingCount       int `json:"finding_count"`
	ClosedFindingCount int `json:"closed_finding_count"`
	MaxElapsedSeconds  int `json:"max_elapsed_seconds"`
	TotalParticipants  int `json:"total_participants"`
}

type ReadinessDossier struct {
	DrillID              string                  `json:"drill_id"`
	BaselineVersion      int                     `json:"baseline_version"`
	SnapshotID           string                  `json:"snapshot_id"`
	Decision             string                  `json:"decision"`
	ReviewerID           string                  `json:"reviewer_id"`
	ReviewReason         string                  `json:"review_reason"`
	ReviewedAt           string                  `json:"reviewed_at"`
	Metrics              DossierMetrics          `json:"metrics"`
	RuleVersion          string                  `json:"rule_version"`
	EventChainHead       string                  `json:"event_chain_head"`
	DocumentDigest       string                  `json:"document_digest"`
	CanonicalPayload     string                  `json:"canonical_payload"`
	ChecklistDigest      string                  `json:"checklist_digest"`
	PassedChecklistCount int                     `json:"passed_checklist_count"`
	ChecklistResults     []ReviewChecklistResult `json:"checklist_results"`
}

type DrillCase struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	SiteName        string              `json:"site_name"`
	ScenarioRisks   []string            `json:"scenario_risks"`
	PlannedStart    string              `json:"planned_start"`
	PlannedEnd      string              `json:"planned_end"`
	CoordinatorID   string              `json:"coordinator_id"`
	State           DrillState          `json:"state"`
	BaselineVersion int                 `json:"baseline_version"`
	RuleVersion     string              `json:"rule_version"`
	Revision        int64               `json:"revision"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	Checkpoints     []Checkpoint        `json:"checkpoints"`
	Activation      *ActivationEvidence `json:"activation,omitempty"`
	Observations    []Observation       `json:"observations"`
	Findings        []Finding           `json:"findings"`
	Receipts        []JudgmentReceipt   `json:"receipts"`
	CurrentSnapshot *ReviewSnapshot     `json:"current_snapshot,omitempty"`
	ReviewHistory   []ReviewRecord      `json:"review_history"`
	Dossier         *ReadinessDossier   `json:"dossier,omitempty"`
}

func ParseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, Invalid("time", "必须使用 RFC3339 时间")
	}
	return t.UTC(), nil
}
