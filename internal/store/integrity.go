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
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
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
