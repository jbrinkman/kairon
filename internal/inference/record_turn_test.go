package inference

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCallRecord_TurnJSON: turn is 1-based, emitted only when non-zero, and
// records written before the field existed still decode.
func TestCallRecord_TurnJSON(t *testing.T) {
	b, err := json.Marshal(CallRecord{Role: "agent", Model: "m", Turn: 2})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if got, ok := m["turn"]; !ok || got != float64(2) {
		t.Errorf("turn = %v (present=%v), want 2 in %s", got, ok, b)
	}

	b, err = json.Marshal(CallRecord{Role: "agent", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"turn"`) {
		t.Errorf("turn must be omitted when zero: %s", b)
	}

	var old CallRecord
	if err := json.Unmarshal([]byte(`{"role":"agent","model":"m","input_tokens":1,"output_tokens":2,"cost_usd":0,"estimated":true,"duration_ms":3}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.Turn != 0 || old.Role != "agent" {
		t.Errorf("legacy record decoded as %+v", old)
	}
}
