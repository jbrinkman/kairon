package provenance

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const docPath = "../../docs/provenance.md"

// Each worked body in docs/provenance.md sits in a fenced block tagged with the
// state the classifier must report for it.
var (
	validFence    = regexp.MustCompile("(?s)```markdown-valid\n(.*?)\n```")
	noneFence     = regexp.MustCompile("(?s)```markdown-none\n(.*?)\n```")
	mismatchFence = regexp.MustCompile("(?s)```markdown-mismatch\n(.*?)\n```")
)

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
		"v1", "sha256", "valid", "none", "mismatch", "routing", "HMAC",
		"Dependencies:", "#377", "#381", "#382",
	}
	for _, term := range terms {
		if !strings.Contains(doc, term) {
			t.Errorf("%s does not mention %q", docPath, term)
		}
	}
}

func TestDocs_ExamplesClassifyAsLabelled(t *testing.T) {
	doc := readDoc(t)
	cases := []struct {
		name  string
		fence *regexp.Regexp
		want  State
	}{
		{"markdown-valid", validFence, StateValid},
		{"markdown-none", noneFence, StateNone},
		{"markdown-mismatch", mismatchFence, StateMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := tc.fence.FindAllStringSubmatch(doc, -1)
			if len(blocks) == 0 {
				t.Fatalf("%s has no %s block", docPath, tc.name)
			}
			for i, m := range blocks {
				if got := Classify(m[1]); got != tc.want {
					t.Errorf("%s block %d classifies %s, want %s:\n%s", tc.name, i+1, got, tc.want, m[1])
				}
			}
		})
	}
}

// The document must say the hash is not a content-trust control. This pins the
// claim so a later edit cannot quietly drop it.
func TestDocs_StatesHashIsNotContentTrust(t *testing.T) {
	doc := strings.Join(strings.Fields(readDoc(t)), " ")
	for _, phrase := range []string{
		"routing and classification signal",
		"not a content-trust control",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("%s does not say %q", docPath, phrase)
		}
	}
}
