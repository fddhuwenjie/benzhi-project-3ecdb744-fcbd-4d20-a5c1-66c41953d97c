package audit

import "testing"

func TestVerifyChainDetectsTampering(t *testing.T) {
	event1, err := NewEvent("d", 1, "created", "a", "2026-01-01T00:00:00Z", "", map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	event2, err := NewEvent("d", 2, "started", "a", "2026-01-01T00:01:00Z", event1.Hash, map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain([]Event{event1, event2}); err != nil {
		t.Fatal(err)
	}
	event2.Payload = []byte(`{"ok":false}`)
	if err := VerifyChain([]Event{event1, event2}); err == nil {
		t.Fatal("expected tamper error")
	}
}

func TestCanonicalJSONSortsObjectKeys(t *testing.T) {
	first, _ := CanonicalJSON(map[string]any{"z": 1, "a": 2})
	second, _ := CanonicalJSON(map[string]any{"a": 2, "z": 1})
	if string(first) != string(second) {
		t.Fatalf("canonical payloads differ: %s / %s", first, second)
	}
}
