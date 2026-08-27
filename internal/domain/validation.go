package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

func ValidateDraft(d DrillCase) error {
	fields := make([]FieldError, 0)
	required := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			fields = append(fields, FieldError{Field: field, Message: "不能为空"})
		}
	}
	required("title", d.Title)
	required("site_name", d.SiteName)
	required("coordinator_id", d.CoordinatorID)
	if len(d.ScenarioRisks) == 0 {
		fields = append(fields, FieldError{Field: "scenario_risks", Message: "至少配置一项场景风险"})
	}
	start, startErr := ParseTime(d.PlannedStart)
	end, endErr := ParseTime(d.PlannedEnd)
	if startErr != nil {
		fields = append(fields, FieldError{Field: "planned_start", Message: "必须使用 RFC3339 时间"})
	}
	if endErr != nil {
		fields = append(fields, FieldError{Field: "planned_end", Message: "必须使用 RFC3339 时间"})
	}
	if startErr == nil && endErr == nil && !end.After(start) {
		fields = append(fields, FieldError{Field: "planned_end", Message: "必须晚于计划开始时间"})
	}
	if len(d.Checkpoints) == 0 {
		fields = append(fields, FieldError{Field: "checkpoints", Message: "至少配置一个检查点"})
	}
	seen := map[int]bool{}
	for index, checkpoint := range d.Checkpoints {
		prefix := "checkpoints[" + itoa(index) + "]"
		if checkpoint.Sequence <= 0 || seen[checkpoint.Sequence] {
			fields = append(fields, FieldError{Field: prefix + ".sequence", Message: "必须是唯一正整数"})
		}
		seen[checkpoint.Sequence] = true
		if strings.TrimSpace(checkpoint.Name) == "" || strings.TrimSpace(checkpoint.ResponsibleRole) == "" {
			fields = append(fields, FieldError{Field: prefix, Message: "名称和责任岗位不能为空"})
		}
		if checkpoint.TimeLimitSeconds <= 0 || checkpoint.MinimumCapacity <= 0 {
			fields = append(fields, FieldError{Field: prefix, Message: "耗时阈值和最低人数必须为正数"})
		}
	}
	for sequence := 1; sequence <= len(d.Checkpoints); sequence++ {
		if !seen[sequence] {
			fields = append(fields, FieldError{Field: "checkpoints.sequence", Message: "顺序必须从 1 连续递增"})
			break
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func ActivationRecords(e ActivationEvidence) []struct {
	Category string
	Record   EvidenceRecord
} {
	return []struct {
		Category string
		Record   EvidenceRecord
	}{
		{"evacuation_route", e.EvacuationRoute},
		{"communication_gear", e.CommunicationGear},
		{"accessible_facility", e.AccessibleFacility},
		{"personnel_ready", e.PersonnelReady},
	}
}

func ValidateActivation(e ActivationEvidence, plannedStart string) error {
	fields := make([]FieldError, 0)
	start, startErr := ParseTime(plannedStart)
	seenDigests := make(map[string]string)
	for _, item := range ActivationRecords(e) {
		prefix := item.Category
		collected, collectedErr := ParseTime(item.Record.CollectedAt)
		if collectedErr != nil {
			fields = append(fields, FieldError{Field: prefix + ".collected_at", Message: "必须使用 RFC3339 时间"})
		}
		validUntil, validErr := ParseTime(item.Record.ValidUntil)
		if validErr != nil {
			fields = append(fields, FieldError{Field: prefix + ".valid_until", Message: "必须使用 RFC3339 时间"})
		}
		if collectedErr == nil && validErr == nil && collected.After(validUntil) {
			fields = append(fields, FieldError{Field: prefix + ".collected_at", Message: "采集时间不得晚于有效截止时间"})
		}
		if startErr == nil && validErr == nil && validUntil.Before(start) {
			fields = append(fields, FieldError{Field: prefix + ".valid_until", Message: "有效截止时间不得早于演练计划开始时间"})
		}
		if err := ValidateDigest(prefix+".content_digest", item.Record.ContentDigest); err != nil {
			var validation *ValidationError
			if errors.As(err, &validation) {
				fields = append(fields, validation.Fields...)
			}
		} else if previous, exists := seenDigests[item.Record.ContentDigest]; exists {
			fields = append(fields, FieldError{Field: prefix + ".content_digest", Message: "不得与 " + previous + " 复用同一内容摘要"})
		} else {
			seenDigests[item.Record.ContentDigest] = prefix
		}
	}
	if strings.TrimSpace(e.RecordedBy) == "" {
		fields = append(fields, FieldError{Field: "recorded_by", Message: "登记人不能为空"})
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func ValidateEvidenceDigest(value string) error {
	return ValidateDigest("evidence_digest", value)
}

func ValidateDigest(field, value string) error {
	if len(value) != sha256.Size*2 {
		return Invalid(field, "必须是 64 位 SHA-256 十六进制摘要")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return Invalid(field, "必须是有效 SHA-256 十六进制摘要")
	}
	return nil
}

func ValidateRemediationPlan(ownerID, correctiveAction, retestPlannedAt, plannedEnd string, now time.Time) error {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(ownerID) == "" {
		fields = append(fields, FieldError{Field: "owner_id", Message: "责任人不能为空"})
	}
	if strings.TrimSpace(correctiveAction) == "" {
		fields = append(fields, FieldError{Field: "corrective_action", Message: "整改措施不能为空"})
	}
	retest, err := ParseTime(retestPlannedAt)
	if err != nil {
		fields = append(fields, FieldError{Field: "retest_planned_at", Message: "必须使用 RFC3339 时间"})
	} else {
		end, endErr := ParseTime(plannedEnd)
		if !retest.After(now.UTC()) {
			fields = append(fields, FieldError{Field: "retest_planned_at", Message: "必须晚于当前业务时间"})
		}
		if endErr == nil && retest.After(end) {
			fields = append(fields, FieldError{Field: "retest_planned_at", Message: "不得晚于演练计划结束时间"})
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func ValidateObservationWindow(d DrillCase, observation Observation) error {
	observed, err := ParseTime(observation.ObservedAt)
	if err != nil {
		return Invalid("observed_at", "必须使用 RFC3339 时间")
	}
	start, _ := ParseTime(d.PlannedStart)
	end, _ := ParseTime(d.PlannedEnd)
	if observed.Before(start) || observed.After(end) {
		return Invalid("observed_at", "现场记录必须位于演练计划时窗内")
	}
	if strings.TrimSpace(observation.ObserverID) == "" {
		return Invalid("observer_id", "观察人不能为空")
	}
	if observation.ParticipantCount < 0 || observation.ElapsedSeconds < 0 {
		return Invalid("metrics", "人数和耗时不能为负数")
	}
	return ValidateEvidenceDigest(observation.EvidenceDigest)
}

func ValidateReviewSeparation(coordinatorID, reviewerID string) error {
	if strings.TrimSpace(reviewerID) == "" {
		return Invalid("reviewer_id", "复核员不能为空")
	}
	if coordinatorID == reviewerID {
		return ErrForbidden
	}
	return nil
}

func NormalizeTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
