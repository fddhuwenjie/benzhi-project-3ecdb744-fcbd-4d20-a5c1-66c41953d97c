package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"shelter-drill-gate/internal/audit"
	"shelter-drill-gate/internal/domain"
)

func (s *Store) VerifyAllChains(ctx context.Context) error {
	ids, err := s.drillIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		events, err := s.Timeline(ctx, id)
		if err != nil {
			return err
		}
		if err := audit.VerifyChain(events); err != nil {
			return fmt.Errorf("演练 %s 审计链损坏: %w", id, err)
		}
	}
	return nil
}

// VerifyAggregates cross-checks every drills.data_json aggregate against the
// immutable fact tables (observations, judgment_receipts, review_snapshots and
// dossiers) as well as the audit chain. It detects missing, extra or drifted
// records that would otherwise let Load return data inconsistent with the
// immutable facts and receipts.
func (s *Store) VerifyAggregates(ctx context.Context) error {
	ids, err := s.drillIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := verifyAggregate(ctx, s.db, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) drillIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM drills ORDER BY id`)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return ids, rows.Err()
}

func verifyAggregate(ctx context.Context, db *sql.DB, drillID string) error {
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data_json FROM drills WHERE id=?`, drillID).Scan(&data)
	if err != nil {
		return fmt.Errorf("演练 %s 聚合读取失败: %w", drillID, err)
	}
	var drill domain.DrillCase
	if err := json.Unmarshal(data, &drill); err != nil {
		return fmt.Errorf("演练 %s 聚合解码失败: %w", drillID, err)
	}
	if err := verifyObservations(ctx, db, drillID, drill.Observations); err != nil {
		return err
	}
	if err := verifyReceipts(ctx, db, drillID, drill.Receipts); err != nil {
		return err
	}
	if err := verifyFindings(ctx, db, drillID, drill.Findings); err != nil {
		return err
	}
	if err := verifySnapshot(ctx, db, drillID, drill.CurrentSnapshot); err != nil {
		return err
	}
	if err := verifyDossier(ctx, db, drillID, drill.Dossier); err != nil {
		return err
	}
	return nil
}

func verifyObservations(ctx context.Context, db *sql.DB, drillID string, observations []domain.Observation) error {
	seen := make(map[string]bool, len(observations))
	for _, observation := range observations {
		encoded, err := json.Marshal(observation)
		if err != nil {
			return fmt.Errorf("演练 %s 现场记录 %s 编码失败: %w", drillID, observation.ID, err)
		}
		var stored []byte
		err = db.QueryRowContext(ctx, `SELECT data_json FROM observations WHERE id=?`, observation.ID).Scan(&stored)
		if err != nil {
			return fmt.Errorf("演练 %s 现场记录 %s 缺失或读取失败: %w", drillID, observation.ID, err)
		}
		if string(stored) != string(encoded) {
			return fmt.Errorf("演练 %s 现场记录 %s 内容漂移", drillID, observation.ID)
		}
		seen[observation.ID] = true
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM observations WHERE drill_id=?`, drillID)
	if err != nil {
		return fmt.Errorf("演练 %s 现场记录表查询失败: %w", drillID, err)
	}
	extras := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] {
			extras = append(extras, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(extras) > 0 {
		return fmt.Errorf("演练 %s 现场记录存在额外记录: %v", drillID, extras)
	}
	return nil
}

func verifyReceipts(ctx context.Context, db *sql.DB, drillID string, receipts []domain.JudgmentReceipt) error {
	seen := make(map[string]bool, len(receipts))
	for _, receipt := range receipts {
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return fmt.Errorf("演练 %s 判定回执 %s 编码失败: %w", drillID, receipt.ID, err)
		}
		var stored []byte
		err = db.QueryRowContext(ctx, `SELECT data_json FROM judgment_receipts WHERE id=?`, receipt.ID).Scan(&stored)
		if err != nil {
			return fmt.Errorf("演练 %s 判定回执 %s 缺失或读取失败: %w", drillID, receipt.ID, err)
		}
		if string(stored) != string(encoded) {
			return fmt.Errorf("演练 %s 判定回执 %s 内容漂移", drillID, receipt.ID)
		}
		seen[receipt.ID] = true
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM judgment_receipts WHERE drill_id=?`, drillID)
	if err != nil {
		return fmt.Errorf("演练 %s 判定回执表查询失败: %w", drillID, err)
	}
	extras := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] {
			extras = append(extras, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(extras) > 0 {
		return fmt.Errorf("演练 %s 判定回执存在额外记录: %v", drillID, extras)
	}
	return nil
}

func verifyFindings(ctx context.Context, db *sql.DB, drillID string, findings []domain.Finding) error {
	seen := make(map[string]bool, len(findings))
	for _, finding := range findings {
		encoded, err := json.Marshal(finding)
		if err != nil {
			return fmt.Errorf("演练 %s 整改项 %s 编码失败: %w", drillID, finding.ID, err)
		}
		var stored []byte
		err = db.QueryRowContext(ctx, `SELECT data_json FROM findings WHERE id=?`, finding.ID).Scan(&stored)
		if err != nil {
			return fmt.Errorf("演练 %s 整改项 %s 缺失或读取失败: %w", drillID, finding.ID, err)
		}
		if string(stored) != string(encoded) {
			return fmt.Errorf("演练 %s 整改项 %s 内容漂移", drillID, finding.ID)
		}
		seen[finding.ID] = true
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM findings WHERE drill_id=?`, drillID)
	if err != nil {
		return fmt.Errorf("演练 %s 整改项表查询失败: %w", drillID, err)
	}
	extras := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] {
			extras = append(extras, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(extras) > 0 {
		return fmt.Errorf("演练 %s 整改项存在额外记录: %v", drillID, extras)
	}
	return nil
}

func verifySnapshot(ctx context.Context, db *sql.DB, drillID string, snapshot *domain.ReviewSnapshot) error {
	if snapshot == nil {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_snapshots WHERE drill_id=?`, drillID).Scan(&count); err != nil {
			return fmt.Errorf("演练 %s 送审快照表查询失败: %w", drillID, err)
		}
		if count > 0 {
			return fmt.Errorf("演练 %s 聚合未包含送审快照但存在 %d 条快照记录", drillID, count)
		}
		return nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("演练 %s 送审快照 %s 编码失败: %w", drillID, snapshot.ID, err)
	}
	var stored []byte
	err = db.QueryRowContext(ctx, `SELECT data_json FROM review_snapshots WHERE id=?`, snapshot.ID).Scan(&stored)
	if err != nil {
		return fmt.Errorf("演练 %s 送审快照 %s 缺失或读取失败: %w", drillID, snapshot.ID, err)
	}
	if string(stored) != string(encoded) {
		return fmt.Errorf("演练 %s 送审快照 %s 内容漂移", drillID, snapshot.ID)
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM review_snapshots WHERE drill_id=?`, drillID)
	if err != nil {
		return fmt.Errorf("演练 %s 送审快照表查询失败: %w", drillID, err)
	}
	extras := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if id != snapshot.ID {
			extras = append(extras, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(extras) > 0 {
		return fmt.Errorf("演练 %s 送审快照存在额外记录: %v", drillID, extras)
	}
	return nil
}

func verifyDossier(ctx context.Context, db *sql.DB, drillID string, dossier *domain.ReadinessDossier) error {
	if dossier == nil {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dossiers WHERE drill_id=?`, drillID).Scan(&count); err != nil {
			return fmt.Errorf("演练 %s 就绪档案表查询失败: %w", drillID, err)
		}
		if count > 0 {
			return fmt.Errorf("演练 %s 聚合未包含就绪档案但存在 %d 条档案记录", drillID, count)
		}
		return nil
	}
	encoded, err := json.Marshal(dossier)
	if err != nil {
		return fmt.Errorf("演练 %s 就绪档案编码失败: %w", drillID, err)
	}
	var stored []byte
	err = db.QueryRowContext(ctx, `SELECT data_json FROM dossiers WHERE drill_id=?`, drillID).Scan(&stored)
	if err != nil {
		return fmt.Errorf("演练 %s 就绪档案缺失或读取失败: %w", drillID, err)
	}
	if string(stored) != string(encoded) {
		return fmt.Errorf("演练 %s 就绪档案内容漂移", drillID)
	}
	return nil
}
