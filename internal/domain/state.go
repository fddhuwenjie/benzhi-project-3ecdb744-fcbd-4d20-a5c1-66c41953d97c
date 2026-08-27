package domain

import "fmt"

var transitions = map[DrillState]map[DrillState]bool{
	StateDraft:       {StateFrozen: true},
	StateFrozen:      {StateActive: true},
	StateActive:      {StateRemediation: true, StateReview: true},
	StateRemediation: {StateReview: true},
	StateReview:      {StateRemediation: true, StateCertified: true},
	StateCertified:   {},
}

func (d *DrillCase) Transition(next DrillState) error {
	if !transitions[d.State][next] {
		return Invalid("state", fmt.Sprintf("不允许从 %s 迁移到 %s", d.State, next))
	}
	d.State = next
	return nil
}

func (d DrillCase) CheckpointByID(id string) (Checkpoint, bool) {
	for _, checkpoint := range d.Checkpoints {
		if checkpoint.ID == id {
			return checkpoint, true
		}
	}
	return Checkpoint{}, false
}

func (d DrillCase) FindingByID(id string) (int, Finding, bool) {
	for index, finding := range d.Findings {
		if finding.ID == id {
			return index, finding, true
		}
	}
	return -1, Finding{}, false
}

func (d DrillCase) HasLiveObservation(checkpointID string) bool {
	for _, observation := range d.Observations {
		if observation.CheckpointID == checkpointID && observation.ObservationKind == "live" {
			return true
		}
	}
	return false
}

func (d DrillCase) NextCheckpoint() (Checkpoint, bool) {
	for _, checkpoint := range d.Checkpoints {
		if !d.HasLiveObservation(checkpoint.ID) {
			return checkpoint, true
		}
	}
	return Checkpoint{}, false
}

func (d DrillCase) AllLiveObservationsSubmitted() bool {
	if len(d.Checkpoints) == 0 {
		return false
	}
	for _, checkpoint := range d.Checkpoints {
		if !d.HasLiveObservation(checkpoint.ID) {
			return false
		}
	}
	return true
}

func (d DrillCase) OpenFindings() []Finding {
	result := make([]Finding, 0)
	for _, finding := range d.Findings {
		if finding.Status != FindingClosed {
			result = append(result, finding)
		}
	}
	return result
}

func (d DrillCase) HasOpenCriticalFinding() bool {
	for _, finding := range d.Findings {
		if finding.Status != FindingClosed && finding.Severity == SeverityCritical {
			return true
		}
	}
	return false
}
