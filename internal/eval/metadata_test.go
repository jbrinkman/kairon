package eval

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// metaEvals writes a one-agent evals dir whose cases are the given file name ->
// YAML body pairs, makes it the active evals dir on the stub backend, and
// returns its path.
func metaEvals(t *testing.T, cases map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeChecksOnlyEvals(t, root, "meta-agent", false, cases)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: root}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	return root
}

// metaCase builds a case file body with the given extra top-level YAML
// (typically a metadata block) appended.
func metaCase(name, extra string) string {
	return "name: " + name + "\ndescription: d\ninput: \"please summarise the repository layout in detail\"\nexpected_output: out\n" +
		"stub:\n  turns:\n    - response: \"## H\"\n" + extra
}

func TestMetadataLoadsScalarTextForms(t *testing.T) {
	metaEvals(t, map[string]string{"tagged.yaml": metaCase("tagged", `metadata:
  skill: cli
  retries: 3
  ratio: 3.0
  hex: 0x10
  quoted: "3"
  flag: true
  upper: True
  caps: FALSE
  off: false
  ts: 2024-01-02
`)})
	cases, err := loadCases("meta-agent")
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	want := CaseMetadata{
		"skill":   "cli",
		"retries": "3",
		"ratio":   "3.0", // as written, not 3
		"hex":     "0x10",
		"quoted":  "3",
		"flag":    "true",
		"upper":   "true", // booleans normalize
		"caps":    "false",
		"off":     "false",
		"ts":      "2024-01-02",
	}
	if !reflect.DeepEqual(cases[0].Metadata, want) {
		t.Errorf("Metadata = %v, want %v", cases[0].Metadata, want)
	}
}

func TestMetadataAbsentLoadsAsBefore(t *testing.T) {
	metaEvals(t, map[string]string{"plain.yaml": metaCase("plain", "")})
	cases, err := loadCases("meta-agent")
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if len(cases) != 1 || cases[0].Metadata != nil {
		t.Fatalf("cases = %+v, want one case with nil Metadata", cases)
	}
	data, err := json.Marshal(cases[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "metadata") {
		t.Errorf("case JSON %s mentions metadata, want it omitted", data)
	}
}

func TestMetadataResolvesAliases(t *testing.T) {
	metaEvals(t, map[string]string{"alias.yaml": metaCase("alias", `metadata:
  first: &v fast
  second: *v
`)})
	cases, err := loadCases("meta-agent")
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if got := cases[0].Metadata["second"]; got != "fast" {
		t.Errorf("alias value = %q, want fast", got)
	}
}

func TestMetadataInvalidFailsLoadCases(t *testing.T) {
	tests := []struct {
		name string
		meta string
		key  string // text the error must carry besides the file name
	}{
		{"list value", "metadata:\n  tags: [a, b]\n", "tags"},
		{"block list value", "metadata:\n  tags:\n    - a\n    - b\n", "tags"},
		{"map value", "metadata:\n  nested:\n    a: b\n", "nested"},
		{"null value", "metadata:\n  empty: null\n", "empty"},
		{"tilde null value", "metadata:\n  empty: ~\n", "empty"},
		{"blank value", "metadata:\n  empty:\n", "empty"},
		{"list metadata", "metadata:\n  - a\n  - b\n", "metadata"},
		{"scalar metadata", "metadata: fast\n", "metadata"},
		{"reserved agent", "metadata:\n  agent: x\n", "agent"},
		{"reserved name", "metadata:\n  name: x\n", "name"},
		{"duplicate key", "metadata:\n  speed: fast\n  speed: slow\n", "speed"},
		{"key with equals", "metadata:\n  \"a=b\": x\n", "a=b"},
		{"key with less", "metadata:\n  \"a<b\": x\n", "a<b"},
		{"key with greater", "metadata:\n  \"a>b\": x\n", "a>b"},
		{"key with bang", "metadata:\n  \"a!b\": x\n", "a!b"},
		{"key with comma", "metadata:\n  \"a,b\": x\n", "a,b"},
		{"empty key", "metadata:\n  \"\": x\n", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaEvals(t, map[string]string{"bad-file.yaml": metaCase("bad", tt.meta)})
			_, err := loadCases("meta-agent")
			if err == nil {
				t.Fatal("loadCases succeeded, want an error")
			}
			if !errors.Is(err, errMalformedCase) {
				t.Errorf("error %v does not wrap errMalformedCase", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "bad-file.yaml") {
				t.Errorf("error %q does not name the case file", msg)
			}
			if !strings.Contains(msg, tt.key) {
				t.Errorf("error %q does not name %q", msg, tt.key)
			}
		})
	}
}

func TestMetadataReservedKeyErrorExplains(t *testing.T) {
	metaEvals(t, map[string]string{"r.yaml": metaCase("r", "metadata:\n  agent: x\n")})
	_, err := loadCases("meta-agent")
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v, want a message saying the key is reserved", err)
	}
}

func TestMetadataValueErrorSaysWhatIsAllowed(t *testing.T) {
	metaEvals(t, map[string]string{"v.yaml": metaCase("v", "metadata:\n  tags: [a]\n")})
	_, err := loadCases("meta-agent")
	if err == nil || !strings.Contains(err.Error(), "string, number or boolean") {
		t.Fatalf("err = %v, want a message naming string, number or boolean", err)
	}
}

func TestMetadataUnknownSiblingFieldStillRejected(t *testing.T) {
	metaEvals(t, map[string]string{"s.yaml": metaCase("s", "metadata:\n  a: b\ntags: [x]\n")})
	_, err := loadCases("meta-agent")
	if err == nil || !errors.Is(err, errMalformedCase) {
		t.Fatalf("err = %v, want errMalformedCase for the unknown field", err)
	}
}

func TestMetadataUnmarshalYAMLDirect(t *testing.T) {
	var tc struct {
		M CaseMetadata `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte("metadata:\n  a: 1\n  b: true\n"), &tc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tc.M, CaseMetadata{"a": "1", "b": "true"}) {
		t.Errorf("M = %v", tc.M)
	}
}

func TestMetadataCaseResultJSON(t *testing.T) {
	with, err := json.Marshal(CaseResult{CaseName: "c", Metadata: CaseMetadata{"speed": "fast"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(with), `"metadata":{"speed":"fast"}`) {
		t.Errorf("tagged result JSON = %s", with)
	}
	without, err := json.Marshal(CaseResult{CaseName: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(without), "metadata") {
		t.Errorf("untagged result JSON = %s, want no metadata field", without)
	}
	var back CaseResult
	if err := json.Unmarshal(with, &back); err != nil {
		t.Fatal(err)
	}
	if back.Metadata["speed"] != "fast" {
		t.Errorf("round trip lost metadata: %+v", back.Metadata)
	}
}

// --- suite_sha256 decision (design 3.7) ---------------------------------

// hashMetaEvals returns the suite hash of meta-agent in the active evals dir.
func hashMetaEvals(t *testing.T) string {
	t.Helper()
	got, err := suiteSHA256("meta-agent")
	if err != nil {
		t.Fatalf("suiteSHA256: %v", err)
	}
	return got
}

func TestSuiteSHA256ChangesOnMetadataOnlyEdit(t *testing.T) {
	root := metaEvals(t, map[string]string{
		"c.yaml": metaCase("c", "metadata:\n  speed: fast\n"),
	})
	before := hashMetaEvals(t)

	// Edit only the metadata value.
	writeCfgFile(t, filepath.Join(root, "cases", "meta-agent", "c.yaml"),
		metaCase("c", "metadata:\n  speed: slow\n"))
	afterValue := hashMetaEvals(t)
	if afterValue == before {
		t.Error("suiteSHA256 unchanged after a metadata-only value edit")
	}

	// Adding a metadata block to a case that had none also changes it.
	writeCfgFile(t, filepath.Join(root, "cases", "meta-agent", "c.yaml"), metaCase("c", ""))
	bare := hashMetaEvals(t)
	writeCfgFile(t, filepath.Join(root, "cases", "meta-agent", "c.yaml"),
		metaCase("c", "metadata:\n  speed: fast\n"))
	if tagged := hashMetaEvals(t); tagged == bare {
		t.Error("suiteSHA256 unchanged after adding a metadata block")
	}
}

func TestSuiteSHA256HashesRawCaseBytes(t *testing.T) {
	body := metaCase("c", "")
	root := metaEvals(t, map[string]string{"c.yaml": body})
	got := hashMetaEvals(t)

	// A case file without metadata hashes exactly as it did before this
	// change: the hash covers the raw bytes of every case file, so a byte
	// edit that does not touch the parsed content still changes it...
	writeCfgFile(t, filepath.Join(root, "cases", "meta-agent", "c.yaml"), body+"# a comment\n")
	if withComment := hashMetaEvals(t); withComment == got {
		t.Error("suiteSHA256 ignored a raw-byte edit: case files must be hashed as raw bytes")
	}

	// ...and restoring the bytes restores the hash.
	writeCfgFile(t, filepath.Join(root, "cases", "meta-agent", "c.yaml"), body)
	if again := hashMetaEvals(t); again != got {
		t.Errorf("hash after restoring bytes = %s, want %s", again, got)
	}

	// The hash input for a case file is the file's raw bytes, framed as a
	// "case" input: pin that directly so a future change to parse-and-
	// reserialize the file (which would rehash every existing suite) fails.
	inputs, err := collectSuiteInputs("meta-agent")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, in := range inputs {
		if in.kind == suiteKindCase && strings.HasSuffix(in.name, "c.yaml") {
			found = true
			raw, err := os.ReadFile(filepath.Join(root, "cases", "meta-agent", "c.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(in.data) != string(raw) {
				t.Errorf("case input data = %q, want the raw file bytes %q", in.data, raw)
			}
		}
	}
	if !found {
		t.Error("no case input found in the suite")
	}
}

// --- metadata does not change execution or scoring ----------------------

func TestMetadataDoesNotChangeExecutionOrScoring(t *testing.T) {
	metaEvals(t, map[string]string{
		"plain.yaml":  metaCase("plain", ""),
		"tagged.yaml": metaCase("tagged", "metadata:\n  skill: cli\n  speed: fast\n  retries: 3\n  flag: true\n"),
	})
	cases, err := loadCases("meta-agent")
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	byName := map[string]TestCase{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	plain, tagged := byName["plain"], byName["tagged"]
	if plain.Metadata != nil || len(tagged.Metadata) != 4 {
		t.Fatalf("fixture cases wrong: plain=%v tagged=%v", plain.Metadata, tagged.Metadata)
	}

	// The agent prompt is built from setup and the opening message only.
	pPlain, err := assemblePrompt(plain.Setup, openingMessage(plain))
	if err != nil {
		t.Fatal(err)
	}
	pTagged, err := assemblePrompt(tagged.Setup, openingMessage(tagged))
	if err != nil {
		t.Fatal(err)
	}
	if pPlain != pTagged {
		t.Errorf("prompts differ: %q vs %q", pPlain, pTagged)
	}

	rubrics, err := loadRubrics("meta-agent")
	if err != nil || len(rubrics) == 0 {
		t.Fatalf("loadRubrics: %v (%d)", err, len(rubrics))
	}
	rubric := rubrics[0]

	var sink strings.Builder
	resPlain := executeCase(rubric, plain, nil, &sink, false)
	resTagged := executeCase(rubric, tagged, nil, &sink, false)

	if resPlain.ActualOutput != resTagged.ActualOutput {
		t.Errorf("outputs differ: %q vs %q", resPlain.ActualOutput, resTagged.ActualOutput)
	}
	if !reflect.DeepEqual(resPlain.Scores, resTagged.Scores) {
		t.Errorf("scores differ:\n plain  %+v\n tagged %+v", resPlain.Scores, resTagged.Scores)
	}
	if len(resPlain.Scores) == 0 {
		t.Fatal("no scores recorded: the comparison proves nothing")
	}
	// The prompt reaches the backend: the estimated input token count is a
	// function of it, so equal counts show the backend saw equal prompts.
	if resPlain.AgentCost.TokensIn != resTagged.AgentCost.TokensIn {
		t.Errorf("agent input tokens differ: %d vs %d", resPlain.AgentCost.TokensIn, resTagged.AgentCost.TokensIn)
	}
	if resPlain.JudgeCost.TokensIn != resTagged.JudgeCost.TokensIn {
		t.Errorf("judge input tokens differ: %d vs %d", resPlain.JudgeCost.TokensIn, resTagged.JudgeCost.TokensIn)
	}
	if resPlain.AgentCost.TokensIn == 0 {
		t.Error("agent input tokens are 0: the backend was not reached")
	}
}
