package domain

import (
	"fmt"
	"strconv"
)

const CurrentRuleVersion = "shelter-rules/1.0"

type RuleFailure struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type Evaluation struct {
	Passed   bool          `json:"passed"`
	Failures []RuleFailure `json:"failures"`
}

func Evaluate(checkpoint Checkpoint, observation Observation) Evaluation {
	failures := make([]RuleFailure, 0, 4)
	if observation.ParticipantCount < checkpoint.MinimumCapacity {
		failures = append(failures, RuleFailure{
			Code: "CAPACITY_MIN", Severity: SeverityCritical,
			Message: fmt.Sprintf("到位人数 %d 低于阈值 %d", observation.ParticipantCount, checkpoint.MinimumCapacity),
		})
	}
	if observation.ElapsedSeconds > checkpoint.TimeLimitSeconds {
		failures = append(failures, RuleFailure{
			Code: "TIME_LIMIT", Severity: SeverityMajor,
			Message: fmt.Sprintf("耗时 %d 秒超过阈值 %d 秒", observation.ElapsedSeconds, checkpoint.TimeLimitSeconds),
		})
	}
	if checkpoint.CommunicationRequired && !observation.CommunicationPassed {
		failures = append(failures, RuleFailure{Code: "COMMUNICATION", Severity: SeverityCritical, Message: "通信联络未通过"})
	}
	if checkpoint.AccessibilityRequired && !observation.AccessibilityPassed {
		failures = append(failures, RuleFailure{Code: "ACCESSIBILITY", Severity: SeverityMajor, Message: "无障碍通行未通过"})
	}
	return Evaluation{Passed: len(failures) == 0, Failures: failures}
}

func FailureForRule(checkpoint Checkpoint, observation Observation, ruleCode string) (RuleFailure, bool) {
	for _, failure := range Evaluate(checkpoint, observation).Failures {
		if failure.Code == ruleCode {
			return failure, true
		}
	}
	return RuleFailure{}, false
}

func BuildJudgmentReceipt(id string, checkpoint Checkpoint, observation Observation, ruleVersion, createdAt string) JudgmentReceipt {
	shortfall := max(checkpoint.MinimumCapacity-observation.ParticipantCount, 0)
	overtime := max(observation.ElapsedSeconds-checkpoint.TimeLimitSeconds, 0)
	items := []JudgmentItem{
		{RuleCode: "CAPACITY_MIN", RuleVersion: ruleVersion, Label: "到位人数", ObservedValue: strconv.Itoa(observation.ParticipantCount), ThresholdValue: strconv.Itoa(checkpoint.MinimumCapacity), Passed: shortfall == 0, Deviation: shortfall, Unit: "人", Severity: SeverityCritical},
		{RuleCode: "TIME_LIMIT", RuleVersion: ruleVersion, Label: "实际耗时", ObservedValue: strconv.Itoa(observation.ElapsedSeconds), ThresholdValue: strconv.Itoa(checkpoint.TimeLimitSeconds), Passed: overtime == 0, Deviation: overtime, Unit: "秒", Severity: SeverityMajor},
		{RuleCode: "COMMUNICATION", RuleVersion: ruleVersion, Label: "通信联络", ObservedValue: strconv.FormatBool(observation.CommunicationPassed), ThresholdValue: strconv.FormatBool(checkpoint.CommunicationRequired), Passed: !checkpoint.CommunicationRequired || observation.CommunicationPassed, Severity: SeverityCritical},
		{RuleCode: "ACCESSIBILITY", RuleVersion: ruleVersion, Label: "无障碍通行", ObservedValue: strconv.FormatBool(observation.AccessibilityPassed), ThresholdValue: strconv.FormatBool(checkpoint.AccessibilityRequired), Passed: !checkpoint.AccessibilityRequired || observation.AccessibilityPassed, Severity: SeverityMajor},
	}
	return JudgmentReceipt{
		ID: id, DrillID: observation.DrillID, CheckpointID: checkpoint.ID, ObservationID: observation.ID,
		CheckpointSequence: checkpoint.Sequence, RuleVersion: ruleVersion, ParticipantShortfall: shortfall,
		OvertimeSeconds: overtime, Items: items, CreatedAt: createdAt,
	}
}
