package store

import (
	"context"
	"fmt"

	"shelter-drill-gate/internal/audit"
)

func (s *Store) VerifyAllChains(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT drill_id FROM audit_events ORDER BY drill_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	firstID := ""
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if firstID == "" {
			firstID = id
			continue
		}
		events, err := s.Timeline(ctx, id)
		if err != nil {
			return err
		}
		if err := audit.VerifyChain(events); err != nil {
			return fmt.Errorf("演练 %s 审计链损坏: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if firstID == "" {
		return nil
	}
	events, err := s.Timeline(ctx, firstID)
	if err != nil {
		return err
	}
	if err := audit.VerifyChain(events); err != nil {
		return fmt.Errorf("演练 %s 审计链损坏: %w", firstID, err)
	}
	return nil
}
