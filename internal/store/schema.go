package store

import (
	"context"
	"fmt"
)

const schema = `
CREATE TABLE IF NOT EXISTS drills (
 id TEXT PRIMARY KEY,
 state TEXT NOT NULL,
 revision INTEGER NOT NULL,
 data_json BLOB NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS observations (
 id TEXT PRIMARY KEY,
 drill_id TEXT NOT NULL REFERENCES drills(id),
 checkpoint_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 data_json BLOB NOT NULL,
 submitted_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS observations_live_checkpoint
 ON observations(drill_id, checkpoint_id) WHERE kind = 'live';
CREATE TABLE IF NOT EXISTS judgment_receipts (
 id TEXT PRIMARY KEY,
 drill_id TEXT NOT NULL REFERENCES drills(id),
 checkpoint_id TEXT NOT NULL,
 observation_id TEXT NOT NULL UNIQUE,
 data_json BLOB NOT NULL,
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS findings (
 id TEXT PRIMARY KEY,
 drill_id TEXT NOT NULL REFERENCES drills(id),
 checkpoint_id TEXT NOT NULL,
 rule_code TEXT NOT NULL,
 status TEXT NOT NULL,
 severity TEXT NOT NULL,
 data_json BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS review_snapshots (
 id TEXT PRIMARY KEY,
 drill_id TEXT NOT NULL REFERENCES drills(id),
 revision INTEGER NOT NULL,
 payload_digest TEXT NOT NULL,
 data_json BLOB NOT NULL,
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS dossiers (
 drill_id TEXT PRIMARY KEY REFERENCES drills(id),
 snapshot_id TEXT NOT NULL,
 document_digest TEXT NOT NULL,
 data_json BLOB NOT NULL,
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_events (
 drill_id TEXT NOT NULL REFERENCES drills(id),
 sequence INTEGER NOT NULL,
 event_type TEXT NOT NULL,
 actor_id TEXT NOT NULL,
 occurred_at TEXT NOT NULL,
 payload_json BLOB NOT NULL,
 previous_hash TEXT NOT NULL,
 event_hash TEXT NOT NULL,
 PRIMARY KEY(drill_id, sequence),
 UNIQUE(drill_id, event_hash)
);
CREATE TABLE IF NOT EXISTS idempotency_requests (
 request_id TEXT PRIMARY KEY,
 drill_id TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 response_json BLOB NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS findings_drill_status ON findings(drill_id, status);
CREATE INDEX IF NOT EXISTS receipts_drill_checkpoint ON judgment_receipts(drill_id, checkpoint_id);
CREATE INDEX IF NOT EXISTS events_drill_sequence ON audit_events(drill_id, sequence);
`

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("迁移 SQLite: %w", err)
	}
	return nil
}
