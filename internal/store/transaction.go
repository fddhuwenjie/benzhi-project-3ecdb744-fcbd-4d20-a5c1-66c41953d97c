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

type CommitInput struct {
	Drill            domain.DrillCase
	ExpectedRevision *int64
	RequestID        string
	Fingerprint      string
	ActorID          string
	EventType        string
	EventPayload     any
	OccurredAt       string
	Response         []byte
}

type CommitResult struct {
	Response []byte
	Replayed bool
	Event    audit.Event
}

func (s *Store) Commit(ctx context.Context, input CommitInput) (result CommitResult, err error) {
	if input.RequestID == "" || input.Fingerprint == "" {
		return result, domain.Invalid("request_id", "request_id 和请求指纹不能为空")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("开始事务: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	if response, fingerprint, found, replayErr := replayTx(ctx, tx, input.RequestID); replayErr != nil {
		return result, replayErr
	} else if found {
		if fingerprint != input.Fingerprint {
			return result, domain.Invalid("request_id", "同一 request_id 对应了不同请求")
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if err = tx.Commit(); err != nil {
			return result, err
		}
		return CommitResult{Response: response, Replayed: true}, nil
	}
	data, err := json.Marshal(input.Drill)
	if err != nil {
		return result, fmt.Errorf("编码演练聚合: %w", err)
	}
	if input.ExpectedRevision == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO drills(id,state,revision,data_json,created_at,updated_at) VALUES(?,?,?,?,?,?)`, input.Drill.ID, input.Drill.State, input.Drill.Revision, data, input.Drill.CreatedAt, input.Drill.UpdatedAt)
	} else {
		var current int64
		if scanErr := tx.QueryRowContext(ctx, `SELECT revision FROM drills WHERE id=?`, input.Drill.ID).Scan(&current); scanErr != nil {
			if errors.Is(scanErr, sql.ErrNoRows) {
				return result, domain.ErrNotFound
			}
			return result, scanErr
		}
		if current != *input.ExpectedRevision {
			return result, &domain.RevisionConflict{Expected: *input.ExpectedRevision, Current: current}
		}
		res, updateErr := tx.ExecContext(ctx, `UPDATE drills SET state=?,revision=?,data_json=?,updated_at=? WHERE id=? AND revision=?`, input.Drill.State, input.Drill.Revision, data, input.Drill.UpdatedAt, input.Drill.ID, *input.ExpectedRevision)
		if updateErr != nil {
			return result, updateErr
		}
		if affected, _ := res.RowsAffected(); affected != 1 {
			return result, &domain.RevisionConflict{Expected: *input.ExpectedRevision, Current: current}
		}
	}
	if err = syncFacts(ctx, tx, input.Drill); err != nil {
		return result, err
	}
	event, err := appendEvent(ctx, tx, input)
	if err != nil {
		return result, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotency_requests(request_id,drill_id,fingerprint,response_json,created_at) VALUES(?,?,?,?,?)`, input.RequestID, input.Drill.ID, input.Fingerprint, input.Response, input.OccurredAt)
	if err != nil {
		return result, fmt.Errorf("保存幂等结果: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("提交事务: %w", err)
	}
	return CommitResult{Response: input.Response, Event: event}, nil
}

func replayTx(ctx context.Context, tx *sql.Tx, requestID string) ([]byte, string, bool, error) {
	var response []byte
	var fingerprint string
	err := tx.QueryRowContext(ctx, `SELECT response_json,fingerprint FROM idempotency_requests WHERE request_id=?`, requestID).Scan(&response, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	return response, fingerprint, true, nil
}

func appendEvent(ctx context.Context, tx *sql.Tx, input CommitInput) (audit.Event, error) {
	var sequence int64
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT sequence,event_hash FROM audit_events WHERE drill_id=? ORDER BY sequence DESC LIMIT 1`, input.Drill.ID).Scan(&sequence, &previous)
	if errors.Is(err, sql.ErrNoRows) {
		sequence, previous, err = 0, "", nil
	}
	if err != nil {
		return audit.Event{}, err
	}
	event, err := audit.NewEvent(input.Drill.ID, sequence+1, input.EventType, input.ActorID, input.OccurredAt, previous, input.EventPayload)
	if err != nil {
		return audit.Event{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(drill_id,sequence,event_type,actor_id,occurred_at,payload_json,previous_hash,event_hash) VALUES(?,?,?,?,?,?,?,?)`, event.DrillID, event.Sequence, event.Type, event.ActorID, event.OccurredAt, []byte(event.Payload), event.PreviousHash, event.Hash)
	if err != nil {
		return audit.Event{}, fmt.Errorf("追加审计事件: %w", err)
	}
	return event, nil
}

func syncFacts(ctx context.Context, tx *sql.Tx, drill domain.DrillCase) error {
	for _, observation := range drill.Observations {
		data, _ := json.Marshal(observation)
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO observations(id,drill_id,checkpoint_id,kind,data_json,submitted_at) VALUES(?,?,?,?,?,?)`, observation.ID, drill.ID, observation.CheckpointID, observation.ObservationKind, data, observation.SubmittedAt)
		if err != nil {
			return fmt.Errorf("保存现场记录: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var existing []byte
			if err := tx.QueryRowContext(ctx, `SELECT data_json FROM observations WHERE id=?`, observation.ID).Scan(&existing); err != nil {
				return err
			}
			if string(existing) != string(data) {
				return fmt.Errorf("不可覆盖现场记录 %s 已发生变化", observation.ID)
			}
		}
	}
	for _, receipt := range drill.Receipts {
		data, _ := json.Marshal(receipt)
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO judgment_receipts(id,drill_id,checkpoint_id,observation_id,data_json,created_at) VALUES(?,?,?,?,?,?)`, receipt.ID, drill.ID, receipt.CheckpointID, receipt.ObservationID, data, receipt.CreatedAt)
		if err != nil {
			return fmt.Errorf("保存判定回执: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var existing []byte
			if err := tx.QueryRowContext(ctx, `SELECT data_json FROM judgment_receipts WHERE id=?`, receipt.ID).Scan(&existing); err != nil {
				return err
			}
			if string(existing) != string(data) {
				return fmt.Errorf("不可覆盖判定回执 %s 已发生变化", receipt.ID)
			}
		}
	}
	for _, finding := range drill.Findings {
		data, _ := json.Marshal(finding)
		_, err := tx.ExecContext(ctx, `INSERT INTO findings(id,drill_id,checkpoint_id,rule_code,status,severity,data_json) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,severity=excluded.severity,data_json=excluded.data_json`, finding.ID, drill.ID, finding.CheckpointID, finding.RuleCode, finding.Status, finding.Severity, data)
		if err != nil {
			return fmt.Errorf("保存整改项: %w", err)
		}
	}
	if drill.CurrentSnapshot != nil {
		data, _ := json.Marshal(drill.CurrentSnapshot)
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO review_snapshots(id,drill_id,revision,payload_digest,data_json,created_at) VALUES(?,?,?,?,?,?)`, drill.CurrentSnapshot.ID, drill.ID, drill.CurrentSnapshot.Revision, drill.CurrentSnapshot.PayloadDigest, data, drill.CurrentSnapshot.CreatedAt)
		if err != nil {
			return fmt.Errorf("保存送审快照: %w", err)
		}
	}
	if drill.Dossier != nil {
		data, _ := json.Marshal(drill.Dossier)
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO dossiers(drill_id,snapshot_id,document_digest,data_json,created_at) VALUES(?,?,?,?,?)`, drill.ID, drill.Dossier.SnapshotID, drill.Dossier.DocumentDigest, data, drill.Dossier.ReviewedAt)
		if err != nil {
			return fmt.Errorf("保存就绪档案: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return fmt.Errorf("已认证档案不可覆盖")
		}
	}
	return nil
}
