package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// simLogRel is the workspace-relative path of the simulated-user decision log.
const simLogRel = ".eval/sim-log.jsonl"

// SimDecision is one routing decision of the simulated user: the agent turn
// whose output was routed, an excerpt of the question, the fact id it was
// routed to (or "none") and that fact's discoverable flag (nil for "none").
type SimDecision struct {
	Turn         int    `json:"turn"`
	Question     string `json:"question"`
	Fact         string `json:"fact"`
	Discoverable *bool  `json:"discoverable,omitempty"`
}

// readSimLog reads <dir>/.eval/sim-log.jsonl: a regular, non-symlink file of
// at most maxCheckFileBytes, one JSON SimDecision per line (blank lines are
// ignored). Any problem is an error whose text is fit for a check detail; the
// fact checks fail on it rather than passing silently.
func readSimLog(dir string) ([]SimDecision, error) {
	abs, info, err := resolveInDir(dir, simLogRel)
	switch {
	case err != nil:
		return nil, fmt.Errorf("sim log unreadable: %w", err)
	case info == nil:
		return nil, errors.New("sim log missing")
	case info.Mode()&os.ModeSymlink != 0:
		return nil, errors.New("sim log is a symlink; refusing to follow it")
	case !info.Mode().IsRegular():
		return nil, errors.New("sim log is not a regular file")
	case info.Size() > maxCheckFileBytes:
		return nil, fmt.Errorf("sim log is larger than the 10 MiB limit (%d bytes)", info.Size())
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("sim log cannot be opened: %w", err)
	}
	defer f.Close()
	// The file must still be the one we inspected (not swapped for a link).
	if fi, err := f.Stat(); err != nil || !os.SameFile(info, fi) {
		return nil, errors.New("sim log changed while being read; refusing")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCheckFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("sim log cannot be read: %w", err)
	}
	if len(data) > maxCheckFileBytes {
		return nil, errors.New("sim log is larger than the 10 MiB limit")
	}

	var decisions []SimDecision
	for i, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var d SimDecision
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, fmt.Errorf("sim log line %d: invalid JSON", i+1)
		}
		if strings.TrimSpace(d.Fact) == "" {
			return nil, fmt.Errorf("sim log line %d: missing fact", i+1)
		}
		decisions = append(decisions, d)
	}
	return decisions, nil
}

// evalFact evaluates fact_asked / fact_not_asked against the sim log.
func evalFact(c Check, in CheckInput) (bool, string) {
	decisions, err := readSimLog(in.Dir)
	if err != nil {
		return false, err.Error()
	}
	var turns []string
	var ids []string
	seen := map[string]bool{}
	for _, d := range decisions {
		if d.Fact == c.Fact {
			turns = append(turns, fmt.Sprint(d.Turn))
		}
		if !seen[d.Fact] {
			seen[d.Fact] = true
			ids = append(ids, d.Fact)
		}
	}
	asked := len(turns) > 0
	switch {
	case c.Type == CheckFactAsked && !asked:
		listed := "none"
		if len(ids) > 0 {
			listed = strings.Join(ids, ", ")
		}
		return false, fmt.Sprintf("fact %s was not asked; asked: %s", c.Fact, listed)
	case c.Type == CheckFactNotAsked && asked:
		word := "turn"
		if len(turns) > 1 {
			word = "turns"
		}
		return false, fmt.Sprintf("fact %s was asked on %s %s", c.Fact, word, strings.Join(turns, ", "))
	}
	return true, ""
}
