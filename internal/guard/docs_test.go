package guard

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const docPath = "../../docs/guards.md"

// yamlFence matches fenced blocks whose info string is exactly "yaml". Blocks
// fenced as "yaml-invalid" (intentionally rejected examples) do not match.
var (
	yamlFence        = regexp.MustCompile("(?s)```yaml\n(.*?)\n```")
	yamlInvalidFence = regexp.MustCompile("(?s)```yaml-invalid\n(.*?)\n```")
)

func readDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	return string(data)
}

func docInput(raw string) Input {
	return Input{Stage: stageoutput.StagePlanner, Raw: raw, Attempt: Attempt{Current: 1, Max: 3}}
}

func TestDocs_NamesRequiredTerms(t *testing.T) {
	doc := readDoc(t)
	terms := []string{
		"Admit", "OnFailure", "AdmissionResult", "FailureContext",
		"Stage", "Guard", "Attempt", "Expected", "Actual",
		"ready to commit", "ErrEmptyChain", "Verify:", "structural",
		"#378", "#379",
	}
	for _, term := range terms {
		if !strings.Contains(doc, term) {
			t.Errorf("%s does not mention %q", docPath, term)
		}
	}
}

func TestDocs_ValidYAMLExamplesAreAdmitted(t *testing.T) {
	doc := readDoc(t)
	blocks := yamlFence.FindAllStringSubmatch(doc, -1)
	if len(blocks) == 0 {
		t.Fatalf("%s has no yaml example", docPath)
	}
	for i, m := range blocks {
		res := NewStructural(nil).Admit(docInput(m[1]))
		if !res.Pass {
			t.Errorf("yaml example %d is not admitted: %+v\n%s", i+1, res.Failure, m[1])
		}
	}
}

func TestDocs_InvalidYAMLExamplesAreRejected(t *testing.T) {
	doc := readDoc(t)
	blocks := yamlInvalidFence.FindAllStringSubmatch(doc, -1)
	if len(blocks) == 0 {
		t.Fatalf("%s has no yaml-invalid example", docPath)
	}
	for i, m := range blocks {
		res := NewStructural(nil).Admit(docInput(m[1]))
		if res.Pass {
			t.Errorf("yaml-invalid example %d is unexpectedly admitted:\n%s", i+1, m[1])
		}
	}
}
