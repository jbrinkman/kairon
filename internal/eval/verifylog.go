package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultIterationsDir is the repo-relative directory holding
	// <agent>/iteration-NN.md logs.
	DefaultIterationsDir = ".kairon/iterations"

	// DefaultMinIterations is the minimum number of iteration logs the CLI
	// requires by default (maturity-model Stage 3: at least two).
	DefaultMinIterations = 2

	// scoreTolerance is the allowed difference, in percent points, between a
	// claimed and a recorded score. It covers the one-decimal rounding of
	// claimed scores.
	scoreTolerance = 0.1

	// scoreEpsilon absorbs float64 representation error so a difference of
	// exactly scoreTolerance (e.g. 60.1 vs 60.0) passes.
	scoreEpsilon = 1e-9
)

// Verification rule IDs. They are stable and documented in
// .kairon/iterations/README.md and docs/evaluation.md.
const (
	ruleNoLogs        = "no-logs"
	ruleFrontMatter   = "front-matter"
	ruleHeadings      = "headings"
	ruleSequence      = "sequence"
	ruleChain         = "chain"
	ruleChronology    = "chronology"
	ruleRunExists     = "run-exists"
	ruleRunData       = "run-data"
	ruleScoreMatch    = "score-match"
	rulePromptChange  = "prompt-change"
	ruleMinIterations = "min-iterations"
)

// ruleOrder ranks rules for stable output ordering within a file.
var ruleOrder = map[string]int{
	ruleNoLogs: 0, ruleFrontMatter: 1, ruleHeadings: 2, ruleSequence: 3,
	ruleChain: 4, ruleChronology: 5, ruleRunExists: 6, ruleRunData: 7,
	ruleScoreMatch: 8, rulePromptChange: 9, ruleMinIterations: 10,
}

const (
	changeTypePrompt = "prompt"
	changeTypeEval   = "eval"
)

var (
	iterationFileRe = regexp.MustCompile(`^iteration-(\d{2,})\.md$`)
	scoreFormatRe   = regexp.MustCompile(`^(100\.0|[0-9]{1,2}\.[0-9])$`)
	// h2HeadingRe matches a CommonMark level-2 ATX heading: up to 3 leading
	// spaces (4+ is a code block), "##" followed by at least one space, and
	// an optional closed-ATX trailing run of # (which must be preceded by a
	// space and is not part of the name). The name capture may be empty for
	// an otherwise-valid empty heading ("##" or "## ##").
	h2HeadingRe = regexp.MustCompile(`^ {0,3}##(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)

	// requiredHeadings are the level-2 headings every log must carry, once
	// each and in this order.
	requiredHeadings = []string{"Baseline", "Hypothesis", "Change", "Results", "Reasoning"}
)

// VerifyLogOptions configures VerifyLog.
type VerifyLogOptions struct {
	IterationsDir string // "" => DefaultIterationsDir
	EvalsDir      string // "" => defaultEvalsDir
	MinIterations int    // used as given (0 disables the minimum); negative is an error
}

// LogViolation is one failed verification rule.
type LogViolation struct {
	File    string // path as joined from IterationsDir (or the agent directory for set-level rules)
	Rule    string // one of the rule IDs
	Message string
}

// VerifyLogError carries every violation found in a log set.
type VerifyLogError struct{ Violations []LogViolation }

// Error renders one "<file>: <rule>: <message>" line per violation.
func (e *VerifyLogError) Error() string {
	lines := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		lines[i] = fmt.Sprintf("%s: %s: %s", v.File, v.Rule, v.Message)
	}
	return strings.Join(lines, "\n")
}

// logFrontMatter is the raw decoded front-matter. Pointers distinguish a
// missing key from a zero value. Scores and the date decode as strings so
// their exact textual format can be enforced.
type logFrontMatter struct {
	Iteration     *string `yaml:"iteration"`
	Date          *string `yaml:"date"`
	ChangeType    *string `yaml:"change_type"`
	BaselineRun   *string `yaml:"baseline_run"`
	ResultRun     *string `yaml:"result_run"`
	BaselineScore *string `yaml:"baseline_score"`
	ResultScore   *string `yaml:"result_score"`
}

// parsedLog is one iteration file after per-file validation.
type parsedLog struct {
	name string // base file name
	path string // as joined from IterationsDir
	num  int    // NN from the file name

	valid         bool // front-matter fully valid; cross-file checks may use the fields below
	iteration     int
	date          time.Time
	changeType    string
	baselineRun   string
	resultRun     string
	baselineScore float64
	resultScore   float64
}

// runRecord is what a results run records for the agent being verified.
type runRecord struct {
	exists      bool
	hasScore    bool
	score       float64 // percent
	promptSHA   string
	scoreSource string
}

// VerifyLog checks <iterations-dir>/<agent>/iteration-NN.md against
// <evals-dir>/results. It collects every violation rather than stopping at the
// first. On success it prints "OK <agent>: N iterations verified" to w and
// returns nil. It never reads package state; directories are explicit.
func VerifyLog(w io.Writer, agent string, opts VerifyLogOptions) error {
	if err := validatePathElement("agent name", agent); err != nil {
		return err
	}
	if opts.MinIterations < 0 {
		return fmt.Errorf("min-iterations must not be negative (got %d)", opts.MinIterations)
	}
	itersDir := opts.IterationsDir
	if itersDir == "" {
		itersDir = DefaultIterationsDir
	}
	evalsDir := opts.EvalsDir
	if evalsDir == "" {
		evalsDir = defaultEvalsDir
	}
	resultsDir := filepath.Join(evalsDir, "results")
	agentDir := filepath.Join(itersDir, agent)

	var violations []LogViolation
	add := func(file, rule, format string, args ...any) {
		violations = append(violations, LogViolation{File: file, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}

	logs := listIterationLogs(agentDir, add)
	if len(logs) == 0 {
		add(agentDir, ruleNoLogs, "no iteration-NN.md files found for agent %q", agent)
	}

	for _, l := range logs {
		parseLogFile(l, add)
	}

	checkSequence(logs, add)
	checkChainAndChronology(logs, add)

	runs := map[string]runRecord{}
	lookup := func(name string) runRecord {
		if r, ok := runs[name]; ok {
			return r
		}
		r := loadRunRecord(resultsDir, name, agent)
		runs[name] = r
		return r
	}
	for _, l := range logs {
		if l.valid {
			checkRuns(l, agent, lookup, add)
		}
	}

	if len(logs) > 0 && len(logs) < opts.MinIterations {
		add(agentDir, ruleMinIterations, "found %d iteration log(s), at least %d required", len(logs), opts.MinIterations)
	}

	if len(violations) > 0 {
		sort.SliceStable(violations, func(i, j int) bool {
			if violations[i].File != violations[j].File {
				return violations[i].File < violations[j].File
			}
			return ruleOrder[violations[i].Rule] < ruleOrder[violations[j].Rule]
		})
		return &VerifyLogError{Violations: violations}
	}
	_, err := fmt.Fprintf(w, "OK %s: %d iterations verified\n", agent, len(logs))
	return err
}

// validatePathElement rejects empty names and anything that is not a single
// path element, so a name can never address a file outside its parent.
func validatePathElement(what, s string) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", what)
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return fmt.Errorf("%s %q must be a single path element (no '/', '\\' or '..')", what, s)
	}
	return nil
}

// listIterationLogs returns the iteration-NN.md files of agentDir sorted by
// (NN, name). Files that look like logs but do not match iteration-NN.md are
// reported as sequence violations; everything else is ignored. A missing
// directory yields no logs.
func listIterationLogs(agentDir string, add func(file, rule, format string, args ...any)) []*parsedLog {
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return nil
	}
	var logs []*parsedLog
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "iteration-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(agentDir, name)
		m := iterationFileRe.FindStringSubmatch(name)
		if m == nil {
			add(path, ruleSequence, "file name %q does not match iteration-NN.md (NN is at least two digits)", name)
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			add(path, ruleSequence, "iteration number in %q is out of range", name)
			continue
		}
		logs = append(logs, &parsedLog{name: name, path: path, num: n})
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].num != logs[j].num {
			return logs[i].num < logs[j].num
		}
		return logs[i].name < logs[j].name
	})
	return logs
}

// parseLogFile validates the front-matter and body of one log, reporting
// front-matter and headings violations and filling in l when valid.
func parseLogFile(l *parsedLog, add func(file, rule, format string, args ...any)) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		add(l.path, ruleFrontMatter, "cannot read file: %v", err)
		return
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	front, body, err := splitFrontMatter(text)
	if err != nil {
		add(l.path, ruleFrontMatter, "%v", err)
	} else {
		parseFrontMatter(l, front, add)
	}
	checkHeadings(l, body, add)
}

// splitFrontMatter splits text into the YAML between the first two '---' lines
// and the remaining body. The file must start with '---'.
func splitFrontMatter(text string) (front, body string, err error) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return "", text, errors.New("file must start with a '---' front-matter delimiter")
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", "", errors.New("front-matter is not closed by a '---' line")
}

// parseFrontMatter validates every required key and, if all are valid, marks
// l valid and records the fields. Each problem is its own violation.
func parseFrontMatter(l *parsedLog, front string, add func(file, rule, format string, args ...any)) {
	var fm logFrontMatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		add(l.path, ruleFrontMatter, "front-matter is not valid YAML: %v", oneLine(err.Error()))
		return
	}
	ok := true
	bad := func(format string, args ...any) {
		ok = false
		add(l.path, ruleFrontMatter, format, args...)
	}

	if fm.Iteration == nil {
		bad("missing required key %q", "iteration")
	} else if n, err := strconv.Atoi(*fm.Iteration); err != nil || n < 0 {
		bad("iteration %q must be a non-negative integer", *fm.Iteration)
	} else {
		l.iteration = n
	}

	if fm.Date == nil {
		bad("missing required key %q", "date")
	} else if d, err := time.Parse("2006-01-02", *fm.Date); err != nil {
		bad("date %q is not a real YYYY-MM-DD calendar date", *fm.Date)
	} else {
		l.date = d
	}

	if fm.ChangeType == nil {
		bad("missing required key %q", "change_type")
	} else if *fm.ChangeType != changeTypePrompt && *fm.ChangeType != changeTypeEval {
		bad("change_type %q must be %q or %q", *fm.ChangeType, changeTypePrompt, changeTypeEval)
	} else {
		l.changeType = *fm.ChangeType
	}

	runField := func(key string, v *string, dst *string) {
		if v == nil {
			bad("missing required key %q", key)
			return
		}
		if err := validatePathElement(key, *v); err != nil {
			bad("%v", err)
			return
		}
		*dst = *v
	}
	runField("baseline_run", fm.BaselineRun, &l.baselineRun)
	runField("result_run", fm.ResultRun, &l.resultRun)

	scoreField := func(key string, v *string, dst *float64) {
		if v == nil {
			bad("missing required key %q", key)
			return
		}
		if !scoreFormatRe.MatchString(*v) {
			bad("%s %q must be a percent with exactly one decimal between 0.0 and 100.0", key, *v)
			return
		}
		f, _ := strconv.ParseFloat(*v, 64) // format already validated
		*dst = f
	}
	scoreField("baseline_score", fm.BaselineScore, &l.baselineScore)
	scoreField("result_score", fm.ResultScore, &l.resultScore)

	l.valid = ok
}

// oneLine flattens a multi-line error so a violation stays on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

// checkHeadings verifies the five required level-2 headings: each exactly
// once, in order, with non-blank content. Headings inside fenced code blocks
// are ignored. It runs even when the front-matter is bad so one run reports
// every problem in the file.
func checkHeadings(l *parsedLog, body string, add func(file, rule, format string, args ...any)) {
	type heading struct {
		name string
		line int
	}
	lines := strings.Split(body, "\n")
	var heads []heading
	var fence string // the opening fence marker while inside a fenced block
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if fence != "" {
			if closesFence(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if f := fenceMarker(trimmed); f != "" && len(line)-len(trimmed) < 4 {
			fence = f
			continue
		}
		if m := h2HeadingRe.FindStringSubmatch(line); m != nil {
			heads = append(heads, heading{name: m[1], line: i})
		}
	}

	// firstAt maps each required heading to the index in heads of its first
	// occurrence.
	firstAt := map[string]int{}
	count := map[string]int{}
	for i, h := range heads {
		count[h.name]++
		if _, seen := firstAt[h.name]; !seen {
			firstAt[h.name] = i
		}
	}

	prev := -1
	prevName := ""
	for _, name := range requiredHeadings {
		switch count[name] {
		case 0:
			add(l.path, ruleHeadings, "missing required heading %q", "## "+name)
			continue
		case 1:
		default:
			add(l.path, ruleHeadings, "heading %q appears %d times, expected exactly once", "## "+name, count[name])
		}
		idx := firstAt[name]
		if idx < prev {
			add(l.path, ruleHeadings, "heading %q is out of order: it must come after %q", "## "+name, "## "+prevName)
		} else {
			prev, prevName = idx, name
		}

		// Content: any non-blank line before the next level-2 heading.
		end := len(lines)
		if idx+1 < len(heads) {
			end = heads[idx+1].line
		}
		blank := true
		for _, ln := range lines[heads[idx].line+1 : end] {
			if strings.TrimSpace(ln) != "" {
				blank = false
				break
			}
		}
		if blank {
			add(l.path, ruleHeadings, "section %q has no content", "## "+name)
		}
	}
}

// fenceMarker returns the run of backticks or tildes (at least three) that
// opens a fenced code block on this line, or "".
func fenceMarker(trimmed string) string {
	for _, c := range []string{"`", "~"} {
		n := 0
		for n < len(trimmed) && trimmed[n] == c[0] {
			n++
		}
		if n >= 3 {
			return strings.Repeat(c, n)
		}
	}
	return ""
}

// closesFence reports whether trimmed closes a block opened by fence: a line
// of only the same character, at least as long as the opening marker.
func closesFence(trimmed, fence string) bool {
	t := strings.TrimRight(trimmed, " \t")
	return len(t) >= len(fence) && t == strings.Repeat(fence[:1], len(t))
}

// checkSequence verifies iteration numbering is exactly 0..N-1 and that each
// front-matter iteration equals its file's NN.
func checkSequence(logs []*parsedLog, add func(file, rule, format string, args ...any)) {
	next := 0
	for i, l := range logs {
		switch {
		case i > 0 && l.num == logs[i-1].num:
			add(l.path, ruleSequence, "duplicate iteration number %d (also used by %s)", l.num, logs[i-1].name)
		case l.num != next:
			add(l.path, ruleSequence, "expected iteration-%02d next but found iteration-%02d (numbering must be contiguous from 00)", next, l.num)
		}
		next = l.num + 1
		if l.valid && l.iteration != l.num {
			add(l.path, ruleSequence, "front-matter iteration %d does not match file number %d", l.iteration, l.num)
		}
	}
}

// checkChainAndChronology compares each log with its predecessor.
func checkChainAndChronology(logs []*parsedLog, add func(file, rule, format string, args ...any)) {
	for i := 1; i < len(logs); i++ {
		prev, cur := logs[i-1], logs[i]
		if !prev.valid || !cur.valid {
			continue
		}
		if cur.baselineRun != prev.resultRun {
			add(cur.path, ruleChain, "baseline_run %q must equal the previous iteration's result_run %q (%s)", cur.baselineRun, prev.resultRun, prev.name)
		}
		if cur.date.Before(prev.date) {
			add(cur.path, ruleChronology, "date %s is earlier than the previous iteration's date %s (%s)", cur.date.Format("2006-01-02"), prev.date.Format("2006-01-02"), prev.name)
		}
	}
}

// checkRuns verifies the runs one valid log references: existence, recorded
// data, score agreement and (for prompt changes) a differing prompt hash.
func checkRuns(l *parsedLog, agent string, lookup func(string) runRecord, add func(file, rule, format string, args ...any)) {
	type ref struct {
		field   string
		run     string
		claimed float64
	}
	refs := []ref{
		{"baseline_score", l.baselineRun, l.baselineScore},
		{"result_score", l.resultRun, l.resultScore},
	}
	needHash := l.changeType == changeTypePrompt

	reported := map[string]bool{} // run name -> already reported run-exists
	usable := true                // both runs exist and have the data needed for prompt-change
	for _, r := range refs {
		rec := lookup(r.run)
		if !rec.exists {
			usable = false
			if !reported[r.run] {
				reported[r.run] = true
				add(l.path, ruleRunExists, "run %q is not a directory under the evals results directory", r.run)
			}
			continue
		}
		if !rec.hasScore {
			usable = false
			add(l.path, ruleRunData, "run %q records no score for agent %q (no agent_verdicts, agent_scores or scoreable cases)", r.run, agent)
		} else if math.Abs(r.claimed-rec.score) > scoreTolerance+scoreEpsilon {
			add(l.path, ruleScoreMatch, "%s %.1f does not match run %q, which recorded %.1f (from %s; tolerance %.1f)", r.field, r.claimed, r.run, rec.score, rec.scoreSource, scoreTolerance)
		}
		if needHash && rec.promptSHA == "" {
			usable = false
			add(l.path, ruleRunData, "run %q records no prompt_sha256 for agent %q, so the prompt change cannot be verified", r.run, agent)
		}
	}

	if needHash && usable {
		base, res := lookup(l.baselineRun), lookup(l.resultRun)
		if base.promptSHA == res.promptSHA {
			add(l.path, rulePromptChange, "change_type is %q but runs %q and %q record the same prompt_sha256 (%s)", changeTypePrompt, l.baselineRun, l.resultRun, base.promptSHA)
		}
	}
}

// loadRunRecord reads what <resultsDir>/<run> records for agent. The run name
// has already been validated as a single path element. Unreadable or absent
// summary/agent files are treated as having no data.
func loadRunRecord(resultsDir, run, agent string) runRecord {
	dir := filepath.Join(resultsDir, run)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return runRecord{}
	}
	rec := runRecord{exists: true}

	summary, sumErr := loadSummary(filepath.Join(dir, "summary.json"))
	if sumErr != nil {
		summary = Summary{}
	}
	result, resErr := loadAgentResult(filepath.Join(dir, agent+".json"))
	if resErr != nil {
		result = AgentResult{}
	}

	switch {
	case hasVerdict(filepath.Join(dir, "summary.json"), summary, agent):
		rec.hasScore, rec.score, rec.scoreSource = true, summary.AgentVerdicts[agent].Score, "agent_verdicts"
	case hasAgentScore(filepath.Join(dir, "summary.json"), summary, agent):
		rec.hasScore, rec.score, rec.scoreSource = true, summary.AgentScores[agent]*100, "agent_scores"
	case resErr == nil:
		if _, max := agentScoreTotals(result); max > 0 {
			rec.hasScore, rec.score, rec.scoreSource = true, agentVerdict(result).Score, agent+".json"
		}
	}

	rec.promptSHA = result.PromptSHA256
	if rec.promptSHA == "" {
		rec.promptSHA = summary.Agents[agent].PromptSHA256
	}
	return rec
}

// hasVerdict reports whether the summary records a *measured* verdict for
// agent. A present-but-empty verdict object ({}) decodes to the zero
// AgentVerdict and is not a real 0.0 score, so it is ignored and the next
// score source is tried. A measured verdict always serializes a "score"
// field (even a real 0.0 as "score":0), so the raw bytes are re-read to tell
// an empty object from a measured verdict rather than relying on a
// non-distinguishing in-memory zero value.
func hasVerdict(summaryPath string, s Summary, agent string) bool {
	if _, ok := s.AgentVerdicts[agent]; !ok {
		return false
	}
	raw, ok := rawSummaryEntry(summaryPath, "agent_verdicts", agent)
	if !ok {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	_, hasScore := fields["score"]
	return hasScore
}

// hasAgentScore reports whether the summary records a *measured* agent_scores
// entry for agent. A JSON null decodes to 0.0 with the key present and is
// indistinguishable from a real zero after unmarshalling, so the raw summary
// bytes are re-read to tell a null entry (ignored) from a real 0.0 (used).
func hasAgentScore(summaryPath string, s Summary, agent string) bool {
	if _, ok := s.AgentScores[agent]; !ok {
		return false
	}
	raw, ok := rawSummaryEntry(summaryPath, "agent_scores", agent)
	return ok && string(raw) != "null"
}

// rawSummaryEntry reads summaryPath and returns the raw JSON value of
// field[agent] (e.g. agent_scores[agent]). ok is false if the file is
// unreadable, not valid JSON, or the key is absent.
func rawSummaryEntry(summaryPath, field, agent string) (json.RawMessage, bool) {
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, false
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(data, &top) != nil {
		return nil, false
	}
	rawField, ok := top[field]
	if !ok {
		return nil, false
	}
	var entries map[string]json.RawMessage
	if json.Unmarshal(rawField, &entries) != nil {
		return nil, false
	}
	v, ok := entries[agent]
	return v, ok
}
