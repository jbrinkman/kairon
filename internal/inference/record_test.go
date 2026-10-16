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

// legacyRecordJSON is the exact marshalled form of a record that carries
// neither step nor source. It pins the pre-existing wire shape byte for byte.
const legacyRecordJSON = `{"role":"agent","model":"m","agent":"a","input_tokens":1,"output_tokens":2,"cost_usd":0.5,"estimated":false,"duration_ms":7}`

func TestCallRecord_StepSourceOmittedWhenEmpty(t *testing.T) {
	b, err := json.Marshal(CallRecord{
		Role: "agent", Model: "m", Agent: "a",
		InputTokens: 1, OutputTokens: 2, CostUSD: 0.5, DurationMS: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != legacyRecordJSON {
		t.Errorf("record without step/source changed shape:\n got=%s\nwant=%s", b, legacyRecordJSON)
	}
}

func TestCallRecord_StepSourceMarshalWhenSet(t *testing.T) {
	var rec CallRecord
	in := `{"role":"agent","model":"m","agent":"a","step":"design","source":"workspace"}`
	if err := json.Unmarshal([]byte(in), &rec); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["step"] != "design" {
		t.Errorf("step = %v, want design: %s", m["step"], b)
	}
	if m["source"] != "workspace" {
		t.Errorf("source = %v, want workspace: %s", m["source"], b)
	}
}

func TestCallRecord_StepSourceRoundTrip(t *testing.T) {
	in := CallRecord{
		Role: "agent", Model: "m", Agent: "selftest-step-a",
		Step: "design", Source: SourceWorkspace,
		InputTokens: 10, OutputTokens: 5, CostUSD: 0.000105, DurationMS: 1,
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
	if SourceWorkspace != "workspace" {
		t.Errorf("SourceWorkspace = %q, want workspace", SourceWorkspace)
	}
}
