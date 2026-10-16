package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jbrinkman/kairon/internal/inference"
)

// stepRecordsRel is the workspace-relative path of the step records a
// workflow agent's dispatch tool writes: one JSON CallRecord per line.
const stepRecordsRel = ".eval/calls.jsonl"

// readStepRecords reads <dir>/.eval/calls.jsonl: an optional, regular,
// non-symlink file of at most maxCheckFileBytes with one JSON CallRecord per
// line (blank lines are skipped but counted in line numbers).
//
// An absent file is normal and yields no records. Every other problem — a
// symlink, a non-regular or oversized file, an unreadable file, or a bad
// line — is an error naming .eval/calls.jsonl (and, for a bad line, its
// 1-based line number). The import is all-or-nothing: on error no records are
// returned. The harness, not the file, decides Source: every accepted record
// is marked inference.SourceWorkspace.
func readStepRecords(dir string) ([]inference.CallRecord, error) {
	abs, info, err := resolveInDir(dir, stepRecordsRel)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s unreadable: %w", stepRecordsRel, err)
	case info == nil:
		return nil, nil
	case info.Mode()&os.ModeSymlink != 0:
		return nil, fmt.Errorf("%s is a symlink; refusing to follow it", stepRecordsRel)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("%s is not a regular file", stepRecordsRel)
	case info.Size() > maxCheckFileBytes:
		return nil, fmt.Errorf("%s is larger than the 10 MiB limit (%d bytes)", stepRecordsRel, info.Size())
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be opened: %w", stepRecordsRel, err)
	}
	defer f.Close()
	// The file must still be the one we inspected (not swapped for a link).
	if fi, err := f.Stat(); err != nil || !os.SameFile(info, fi) {
		return nil, fmt.Errorf("%s changed while being read; refusing", stepRecordsRel)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCheckFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", stepRecordsRel, err)
	}
	if len(data) > maxCheckFileBytes {
		return nil, fmt.Errorf("%s is larger than the 10 MiB limit", stepRecordsRel)
	}

	var recs []inference.CallRecord
	for i, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		rec, err := parseStepRecord(line)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", stepRecordsRel, i+1, err)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// parseStepRecord decodes and validates one step-record line. Unknown fields
// are tolerated (and dropped) so the writer can add fields without breaking
// the harness.
func parseStepRecord(line []byte) (inference.CallRecord, error) {
	var rec inference.CallRecord
	dec := json.NewDecoder(bytes.NewReader(line))
	if err := dec.Decode(&rec); err != nil {
		return rec, fmt.Errorf("invalid JSON record: %v", err)
	}
	if dec.More() {
		return rec, fmt.Errorf("invalid JSON record: unexpected data after the record")
	}
	if strings.TrimSpace(rec.Step) == "" {
		return rec, fmt.Errorf("missing step")
	}
	if strings.TrimSpace(rec.Agent) == "" {
		return rec, fmt.Errorf("missing agent")
	}
	// Imported cost is agent cost; a judge record would be misfiled.
	switch rec.Role {
	case "":
		rec.Role = "agent"
	case "agent":
	default:
		return rec, fmt.Errorf("role must be \"agent\", got %q", rec.Role)
	}
	// A workspace must not be able to lower the case's cost.
	switch {
	case rec.InputTokens < 0:
		return rec, fmt.Errorf("negative input_tokens (%d)", rec.InputTokens)
	case rec.OutputTokens < 0:
		return rec, fmt.Errorf("negative output_tokens (%d)", rec.OutputTokens)
	case rec.CostUSD < 0:
		return rec, fmt.Errorf("negative cost_usd (%v)", rec.CostUSD)
	case rec.DurationMS < 0:
		return rec, fmt.Errorf("negative duration_ms (%d)", rec.DurationMS)
	}
	rec.Source = inference.SourceWorkspace
	return rec, nil
}

// importStepRecords reads the workspace's step records and, on success,
// appends them to cr.Calls and adds their tokens and cost to cr.AgentCost. On
// error nothing is imported and cr is left unchanged.
func importStepRecords(cr *CaseResult, dir string) error {
	recs, err := readStepRecords(dir)
	if err != nil {
		return err
	}
	for _, r := range recs {
		source := string(inference.UsageReported)
		if r.Estimated {
			source = string(inference.UsageEstimated)
		}
		cr.Calls = append(cr.Calls, r)
		cr.AgentCost.Add(CostInfo{
			TokensIn:     r.InputTokens,
			TokensOut:    r.OutputTokens,
			EstimatedUSD: r.CostUSD,
			Model:        r.Model,
			UsageSource:  source,
		})
	}
	return nil
}
