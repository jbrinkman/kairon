package inference

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCallRecord_JSONFieldNames(t *testing.T) {
	rec := CallRecord{
		Role: "agent", Model: "m", Agent: "a", Criterion: "c",
		InputTokens: 1, OutputTokens: 2, CostUSD: 0.5, Estimated: true,
		DurationMS: 7, PromptSHA256: "abc", Error: "boom",
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"role", "model", "agent", "criterion", "input_tokens", "output_tokens",
		"cost_usd", "estimated", "duration_ms", "prompt_sha256", "error",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing JSON field %q in %s", k, b)
		}
	}
	if len(m) != 11 {
		t.Errorf("got %d fields, want 11: %s", len(m), b)
	}
}

func TestCallRecord_RoundTrip(t *testing.T) {
	in := CallRecord{
		Role: "judge", Model: "claude-sonnet-5.5", Criterion: "clarity",
		InputTokens: 120, OutputTokens: 30, CostUSD: 0.0123, Estimated: false,
		DurationMS: 1500, Error: "timeout",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out CallRecord
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestCallRecord_OmitEmptyOptionalFields(t *testing.T) {
	b, err := json.Marshal(CallRecord{Role: "judge", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"agent", "criterion", "prompt_sha256", "error"} {
		if _, ok := m[k]; ok {
			t.Errorf("field %q should be omitted when empty: %s", k, b)
		}
	}
	// Numeric/bool fields are always present, even when zero/false.
	for _, k := range []string{"input_tokens", "output_tokens", "cost_usd", "estimated", "duration_ms"} {
		if _, ok := m[k]; !ok {
			t.Errorf("field %q must always be present: %s", k, b)
		}
	}
}
