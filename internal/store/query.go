package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"shelter-drill-gate/internal/audit"
	"shelter-drill-gate/internal/domain"
)

func (s *Store) Load(ctx context.Context, id string) (domain.DrillCase, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data_json FROM drills WHERE id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DrillCase{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DrillCase{}, err
	}
	var drill domain.DrillCase
	if err := json.Unmarshal(data, &drill); err != nil {
		return drill, fmt.Errorf("解码演练聚合: %w", err)
	}
	return drill, nil
}

func (s *Store) List(ctx context.Context) ([]domain.DrillCase, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data_json FROM drills ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.DrillCase, 0)
	for rows.Next() {
		var data []byte
		var drill domain.DrillCase
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &drill); err != nil {
			return nil, err
		}
		result = append(result, drill)
	}
	return result, rows.Err()
}

func (s *Store) Replay(ctx context.Context, requestID, fingerprint string) ([]byte, bool, error) {
	var response []byte
	var storedFingerprint string
	err := s.db.QueryRowContext(ctx, `SELECT response_json,fingerprint FROM idempotency_requests WHERE request_id=?`, requestID).Scan(&response, &storedFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if storedFingerprint != fingerprint {
		return nil, false, domain.Invalid("request_id", "同一 request_id 对应了不同请求")
	}
	return response, true, nil
}

func (s *Store) Timeline(ctx context.Context, drillID string) ([]audit.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,event_type,actor_id,occurred_at,payload_json,previous_hash,event_hash FROM audit_events WHERE drill_id=? ORDER BY sequence`, drillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]audit.Event, 0)
	for rows.Next() {
		event := audit.Event{DrillID: drillID}
		if err := rows.Scan(&event.Sequence, &event.Type, &event.ActorID, &event.OccurredAt, &event.Payload, &event.PreviousHash, &event.Hash); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ChainHead(ctx context.Context, drillID string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT event_hash FROM audit_events WHERE drill_id=? ORDER BY sequence DESC LIMIT 1`, drillID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}
