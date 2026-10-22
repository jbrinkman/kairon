package failure

import (
	"os"
	"strings"
	"testing"
)

const docPath = "../../docs/failure-contract.md"

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
		"Tier 1", "Tier 2", "punch-out", "FailureContext", "terminal-pending",
		"run id", "marker", "max_retries", "max_qa_retries", "Reconcile",
		"Route", "Runner", "PunchOut", "Decide", "kairon-failed",
		"comment before the label", "remove the label", "idempotent",
		"#381", "#382",
	}
	for _, term := range terms {
		if !strings.Contains(doc, term) {
			t.Errorf("%s does not mention %q", docPath, term)
		}
	}
}

func TestDocs_HasNoYAMLFence(t *testing.T) {
	if strings.Contains(readDoc(t), "```yaml") {
		t.Errorf("%s must not contain a yaml fence", docPath)
	}
}
