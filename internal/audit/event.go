package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Event struct {
	DrillID      string          `json:"drill_id"`
	Sequence     int64           `json:"sequence"`
	Type         string          `json:"type"`
	ActorID      string          `json:"actor_id"`
	OccurredAt   string          `json:"occurred_at"`
	Payload      json.RawMessage `json:"payload"`
	PreviousHash string          `json:"previous_hash"`
	Hash         string          `json:"hash"`
}

type eventEnvelope struct {
	DrillID      string          `json:"drill_id"`
	Sequence     int64           `json:"sequence"`
	Type         string          `json:"type"`
	ActorID      string          `json:"actor_id"`
	OccurredAt   string          `json:"occurred_at"`
	Payload      json.RawMessage `json:"payload"`
	PreviousHash string          `json:"previous_hash"`
}

type VerificationError struct {
	Sequence int64
	Phase    string
	Detail   string
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("审计事件 %d %s: %s", e.Sequence, e.Phase, e.Detail)
}

func verificationError(sequence int64, phase string, err error) error {
	return &VerificationError{Sequence: sequence, Phase: phase, Detail: err.Error()}
}

func NewEvent(drillID string, sequence int64, eventType, actorID, occurredAt, previousHash string, payload any) (Event, error) {
	canonical, err := CanonicalJSON(payload)
	if err != nil {
		return Event{}, fmt.Errorf("规范化审计载荷: %w", err)
	}
	event := Event{DrillID: drillID, Sequence: sequence, Type: eventType, ActorID: actorID, OccurredAt: occurredAt, Payload: canonical, PreviousHash: previousHash}
	digest, err := eventDigest(event)
	if err != nil {
		return Event{}, err
	}
	event.Hash = digest
	return event, nil
}

func eventDigest(event Event) (string, error) {
	envelope := eventEnvelope{DrillID: event.DrillID, Sequence: event.Sequence, Type: event.Type, ActorID: event.ActorID, OccurredAt: event.OccurredAt, Payload: event.Payload, PreviousHash: event.PreviousHash}
	canonical, err := CanonicalJSON(envelope)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func VerifyChain(events []Event) error {
	previous := ""
	for index, event := range events {
		expectedSequence := int64(index + 1)
		if event.Sequence != expectedSequence {
			return fmt.Errorf("审计事件缺号：期望 %d，实际 %d", expectedSequence, event.Sequence)
		}
		if event.PreviousHash != previous {
			return fmt.Errorf("审计事件 %d 前序摘要不匹配", event.Sequence)
		}
		digest, err := eventDigest(event)
		if err != nil {
			return verificationError(event.Sequence, "无法计算摘要", err)
		}
		if digest != event.Hash {
			return fmt.Errorf("审计事件 %d 摘要不匹配", event.Sequence)
		}
		previous = event.Hash
	}
	return nil
}
