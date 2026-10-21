package stageoutput

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const docPath = "../../docs/stage-output.md"

// yamlFence matches fenced blocks whose info string is exactly "yaml". Blocks
// fenced as "yaml-invalid" (intentionally rejected examples) do not match.
var yamlFence = regexp.MustCompile("(?s)```yaml\n(.*?)\n```")

func readDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	return string(data)
}

func TestDocs_NamesRequiredTerms(t *testing.T) {
	doc := readDoc(t)
	terms := []string{
		"schema_version", "stage", "result", "source", "kind", "ref",
		"generated", "body", "unresolved", "inferences",
		"canonical", "punch_out",
		"YAML", "yaml.v3", "KnownFields(true)", "kiro-plan",
		"ErrUnknownVersion", "ErrInvalidDocument",
		"version bump", "JSON",
	}
	for _, term := range terms {
		if !strings.Contains(doc, term) {
			t.Errorf("%s does not mention %q", docPath, term)
		}
	}
}

func TestDocs_ValidYAMLExamplesParse(t *testing.T) {
	doc := readDoc(t)
	blocks := yamlFence.FindAllStringSubmatch(doc, -1)

	results := map[Result]bool{}
	for i, m := range blocks {
		d, err := Parse([]byte(m[1]))
		if err != nil {
			t.Errorf("yaml example %d does not parse: %v\n%s", i+1, err, m[1])
			continue
		}
		results[d.Result] = true
	}
	for _, r := range []Result{ResultCanonical, ResultPunchOut} {
		if !results[r] {
			t.Errorf("%s has no valid %s yaml example", docPath, r)
		}
	}
}

func TestDocs_InvalidExamplesAreRejected(t *testing.T) {
	doc := readDoc(t)
	blocks := regexp.MustCompile("(?s)```yaml-invalid\n(.*?)\n```").FindAllStringSubmatch(doc, -1)
	for i, m := range blocks {
		if _, err := Parse([]byte(m[1])); err == nil {
			t.Errorf("yaml-invalid example %d unexpectedly parses:\n%s", i+1, m[1])
		}
	}
}
