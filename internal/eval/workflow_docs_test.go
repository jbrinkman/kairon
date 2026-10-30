package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

// The documented algorithm, implemented independently here from the doc's
// words, must produce what the code produces: the documented framing, order
// (dispatching agent first, then sub-agents sorted by name) and part kinds.
func TestWorkflowDocsAlgorithmMatchesCode(t *testing.T) {
	frame := func(kind, data string) string {
		return kind + "\x00" + strconv.Itoa(len(data)) + "\x00" + data
	}
	subs := map[string]SubAgentProvenance{
		"zeta":  {PromptSHA256: strings.Repeat("b", 64)},
		"alpha": {PromptSHA256: strings.Repeat("a", 64)},
	}
	dispatcherSHA := strings.Repeat("0", 64)

	var b strings.Builder
	b.WriteString(frame("workflow-agent", "krew"))
	b.WriteString(frame("prompt-sha256", dispatcherSHA))
	b.WriteString(frame("workflow-agent", "alpha"))
	b.WriteString(frame("prompt-sha256", subs["alpha"].PromptSHA256))
	b.WriteString(frame("workflow-agent", "zeta"))
	b.WriteString(frame("prompt-sha256", subs["zeta"].PromptSHA256))
	sum := sha256.Sum256([]byte(b.String()))
	want := hex.EncodeToString(sum[:])

	if got := workflowSHA256("krew", dispatcherSHA, subs); got != want {
		t.Errorf("workflowSHA256 = %s, documented algorithm gives %s", got, want)
	}
}
