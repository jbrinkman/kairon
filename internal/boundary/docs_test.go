package boundary

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/provenance"
)

const docPath = "../../docs/boundary.md"

// Each worked input document in docs/boundary.md sits in a fenced block tagged
// yaml-input.
var inputFence = regexp.MustCompile("(?s)```yaml-input\n(.*?)\n```")

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
		"transport", "load-and-verify", "valid", "none", "mismatch",
		"terminal-pending", "untrusted", "KAIRON_INPUT_DOCUMENT",
		"#378", "#379", "#381", "#397",
	}
	for _, term := range terms {
		if !strings.Contains(doc, term) {
			t.Errorf("%s does not mention %q", docPath, term)
		}
	}
}

func TestDocs_InputExamplesParse(t *testing.T) {
	doc := readDoc(t)
	blocks := inputFence.FindAllStringSubmatch(doc, -1)
	if len(blocks) == 0 {
		t.Fatalf("%s has no yaml-input block", docPath)
	}
	for i, m := range blocks {
		d, err := ParseInputDocument([]byte(m[1] + "\n"))
		if err != nil {
			t.Errorf("yaml-input block %d does not parse: %v\n%s", i+1, err, m[1])
			continue
		}
		// ParseInputDocument already checks this; assert it here so the
		// requirement that the example hash comes from provenance.Hash is
		// stated in the test itself.
		if got := provenance.Hash(d.Body); d.Provenance.SHA256 != got {
			t.Errorf("yaml-input block %d sha256 is %s, provenance.Hash(body) is %s", i+1, d.Provenance.SHA256, got)
		}
	}
}
