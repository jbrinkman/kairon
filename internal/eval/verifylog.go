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
	ruleRunComplete   = "run-complete"
	ruleRunConfig     = "run-config"
	ruleScoreMatch    = "score-match"
	rulePromptChange  = "prompt-change"
	ruleSuiteChange   = "suite-change"
	rulePhase         = "phase"
	ruleRed           = "red"
	ruleGreen         = "green"
	ruleRefactor      = "refactor"
	ruleCycle         = "cycle"
	ruleMinIterations = "min-iterations"
)

// ruleOrder ranks rules for stable output ordering within a file.
var ruleOrder = map[string]int{
	ruleNoLogs: 0, ruleFrontMatter: 1, ruleHeadings: 2, ruleSequence: 3,
	ruleChain: 4, ruleChronology: 5, ruleRunExists: 6, ruleRunData: 7,
	ruleRunComplete: 8, ruleRunConfig: 9, ruleScoreMatch: 10,
	rulePromptChange: 11, ruleSuiteChange: 12, rulePhase: 13, ruleRed: 14,
	ruleGreen: 15, ruleRefactor: 16, ruleCycle: 17, ruleMinIterations: 18,
}

const (
	changeTypePrompt = "prompt"
	changeTypeEval   = "eval"
)

// Iteration phases (Eval-Driven Development steps). baseline is allowed only
// in iteration-00.md.
const (
	phaseBaseline = "baseline"
	phaseRed      = "red"
	phaseGreen    = "green"
	phaseRefactor = "refactor"
)

// phaseValues lists the allowed phase values in documentation order.
var phaseValues = []string{phaseBaseline, phaseRed, phaseGreen, phaseRefactor}

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
	// RequireCycle, used as given (zero value false), requires the set to
	// contain a red iteration followed later by a green iteration and at
	// least one refactor iteration.
	RequireCycle bool
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
// their exact textual format can be enforced. Phase decodes into a yaml.Node
// (not *string) so a non-scalar value is reported as a bad phase rather than
// aborting the whole unmarshal; an absent key leaves it a zero node (Kind 0).
type logFrontMatter struct {
	Iteration     *string   `yaml:"iteration"`
	Date          *string   `yaml:"date"`
	ChangeType    *string   `yaml:"change_type"`
	BaselineRun   *string   `yaml:"baseline_run"`
	ResultRun     *string   `yaml:"result_run"`
	BaselineScore *string   `yaml:"baseline_score"`
	ResultScore   *string   `yaml:"result_score"`
	Phase         yaml.Node `yaml:"phase"`
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

	// phase is the front-matter phase when it is one of phaseValues (and, for
	// baseline, the file is iteration-00.md); "" otherwise. A bad phase never
	// clears valid.
	phase string
}

// runRecord is what a results run records for the agent being verified.
//
// Every fact other than the score is "" (or !hasComplete) when the run does not
// record it. For comparison "not recorded" is a distinct value: it differs
// from any recorded value and equals another "not recorded".
type runRecord struct {
	exists      bool
	hasScore    bool
	score       float64  // percent
	successRate *float64 // attempt success rate (percent); nil when not recorded
	promptSHA   string
	scoreSource string

	agentModel  string
	judgeModel  string
	sandbox     string // "native", "container" or ""
	suiteSHA    string
	complete    bool
	hasComplete bool
	caseFilter  string

	// checks is the per-check evidence read from <agent>.json.
	checks checkEvidence
}

// checkID identifies a check across runs: the case, the criterion and the
// check's label without its "#<index> " prefix, so inserting a check earlier in
// a case does not make the later checks look new. Fields the label does not
// print are not part of the identity.
type checkID struct {
	caseName  string
	criterion string
	label     string
}

func (id checkID) String() string {
	return fmt.Sprintf("%s / %s / %s", id.caseName, id.criterion, id.label)
}

// checkEvidence is what one run's <agent>.json records about its checks.
type checkEvidence struct {
	repeats map[string]int   // case name -> number of recorded repeats (CaseResult entries)
	present map[checkID]int  // identity -> number of repeats that record it
	failed  map[checkID]bool // identity -> failed in at least one repeat
	order   []checkID        // identities in first-seen order, for stable messages
	seen    map[checkID]bool // membership of order
}

var checkIndexPrefixRe = regexp.MustCompile(`^#\d+ `)

// loadCheckEvidence groups result.Cases by case name (each CaseResult is one
// repeat) and records every check under its identity. Skipped criteria
// contribute nothing.
func loadCheckEvidence(result AgentResult) checkEvidence {
	ev := checkEvidence{
		repeats: map[string]int{},
		present: map[checkID]int{},
		failed:  map[checkID]bool{},
		seen:    map[checkID]bool{},
	}
	for _, c := range result.Cases {
		ev.repeats[c.CaseName]++
		inRepeat := map[checkID]bool{}
		for _, sc := range c.Scores {
			if sc.Skipped {
				continue
			}
			for _, ck := range sc.Checks {
				id := checkID{c.CaseName, sc.Name, checkIndexPrefixRe.ReplaceAllString(ck.Label, "")}
				if !ev.seen[id] {
					ev.seen[id] = true
					ev.order = append(ev.order, id)
				}
				inRepeat[id] = true
				if !ck.Passed {
					ev.failed[id] = true
				}
			}
		}
		for id := range inRepeat {
			ev.present[id]++
		}
	}
	return ev
}

// failing lists, in first-seen order, the checks that fail in any repeat.
func (ev checkEvidence) failing() []checkID {
	var out []checkID
	for _, id := range ev.order {
		if ev.failed[id] {
			out = append(out, id)
		}
	}
	return out
}

// passesEverywhere reports whether id is recorded in every repeat of its case
// and passes in all of them. A check absent from a repeat is not a pass.
func (ev checkEvidence) passesEverywhere(id checkID) bool {
	n := ev.repeats[id.caseName]
	return n > 0 && ev.present[id] == n && !ev.failed[id]
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

	if opts.RequireCycle && len(logs) > 0 {
		checkCycle(agentDir, logs, add)
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
	checkPhaseKey(l, &fm.Phase, add)
}

// checkPhaseKey validates the phase key. A missing or bad phase is its own
// rule and deliberately does not clear l.valid, so the chain, run and score
// checks of the same file still run. The field decodes into a yaml.Node so a
// non-scalar value (a list or map) is reported here as a bad phase rather than
// aborting front-matter parsing and masking the file's other violations.
func checkPhaseKey(l *parsedLog, phase *yaml.Node, add func(file, rule, format string, args ...any)) {
	allowed := strings.Join(phaseValues, ", ")
	value, scalar := phaseScalar(phase)
	switch {
	case phase.Kind == 0:
		add(l.path, rulePhase, "missing required key %q (allowed values: %s)", "phase", allowed)
	case !scalar || !isPhaseValue(value):
		add(l.path, rulePhase, "phase %q must be one of %s", value, allowed)
	case value == phaseBaseline && l.num != 0:
		add(l.path, rulePhase, "phase %q is allowed only in iteration-00.md, not %s", phaseBaseline, l.name)
	default:
		l.phase = value
	}
}

// phaseScalar returns the string value of a scalar phase node and whether the
// node is a non-null scalar. A list, map or null node is not a usable phase
// value; its rendered form is returned so the violation message can quote it.
// A zero node (absent key, Kind 0) is handled by the caller before this.
func phaseScalar(n *yaml.Node) (value string, scalar bool) {
	if n.Kind == yaml.ScalarNode && n.Tag != "!!null" {
		return n.Value, true
	}
	return renderPhaseNode(n), false
}

// renderPhaseNode produces a short single-line rendering of a non-scalar phase
// node for use in a violation message.
func renderPhaseNode(n *yaml.Node) string {
	out, err := yaml.Marshal(n)
	if err != nil {
		return ""
	}
	return oneLine(string(out))
}

// isPhaseValue reports whether s is one of the allowed phase values.
func isPhaseValue(s string) bool {
	for _, v := range phaseValues {
		if s == v {
			return true
		}
	}
	return false
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
	var fence fenceTracker
	for i, line := range lines {
		if fence.skip(line) {
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

// fenceTracker follows CommonMark fenced code blocks line by line. The zero
// value is outside a fence. It is shared by verify-log and audit.
type fenceTracker struct {
	marker string // the opening fence marker while inside a fenced block
}

// skip feeds the next line and reports whether it is a fence delimiter or
// inside a fenced block, i.e. not ordinary prose. An unclosed fence runs to
// the end of the input.
func (f *fenceTracker) skip(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if f.marker != "" {
		if closesFence(trimmed, f.marker) {
			f.marker = ""
		}
		return true
	}
	if m := fenceMarker(trimmed); m != "" && len(line)-len(trimmed) < 4 {
		f.marker = m
		return true
	}
	return false
}

// fenceMarker returns the run of backticks or tildes (at least three) that
// opens a fenced code block on this line, or "".
//
// A backtick opener's info string may not contain a backtick (CommonMark
// §119), so a line like ```foo``` is an inline code span, not a fence
// opener, and must not open a block. Tilde info strings may contain
// backticks, so tilde openers are unaffected.
func fenceMarker(trimmed string) string {
	for _, c := range []string{"`", "~"} {
		n := 0
		for n < len(trimmed) && trimmed[n] == c[0] {
			n++
		}
		if n >= 3 {
			if c == "`" && strings.ContainsRune(trimmed[n:], '`') {
				return ""
			}
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

// orNotRecorded renders an empty fact for a message.
func orNotRecorded(v string) string {
	if v == "" {
		return notRecorded
	}
	return v
}

// checkRuns verifies the runs one valid log references: existence, recorded
// data, completeness, score agreement, and that the two runs differ the way
// the change type claims (prompt hash, suite hash, judge) and agree on
// everything else (models, sandbox).
//
// Each pairwise check runs whenever both runs record the facts it needs; a
// missing fact is reported once by run-data and does not cascade.
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

	reported := map[string]bool{} // run name -> already reported run-exists
	bothExist := true
	for _, r := range refs {
		rec := lookup(r.run)
		if !rec.exists {
			bothExist = false
			if !reported[r.run] {
				reported[r.run] = true
				add(l.path, ruleRunExists, "run %q is not a directory under the evals results directory", r.run)
			}
			continue
		}
		if !rec.hasScore {
			add(l.path, ruleRunData, "run %q records no score for agent %q (no agent_verdicts, agent_scores or scoreable cases)", r.run, agent)
		} else if math.Abs(r.claimed-rec.score) > scoreTolerance+scoreEpsilon {
			add(l.path, ruleScoreMatch, "%s %.1f does not match run %q, which recorded %.1f (from %s; tolerance %.1f)", r.field, r.claimed, r.run, rec.score, rec.scoreSource, scoreTolerance)
		}

		var missing []string
		if rec.promptSHA == "" {
			missing = append(missing, "prompt_sha256")
		}
		if rec.suiteSHA == "" {
			missing = append(missing, "suite_sha256")
		}
		if !rec.hasComplete {
			missing = append(missing, "complete")
		}
		if len(missing) > 0 {
			add(l.path, ruleRunData, "run %q records no %s for agent %q, so the iteration cannot be verified", r.run, strings.Join(missing, ", "), agent)
		}
		if rec.hasComplete && !rec.complete {
			add(l.path, ruleRunComplete, "run %q is not a complete run of agent %q (complete=false, case_filter=%q); only full runs can be compared", r.run, agent, rec.caseFilter)
		}
	}
	if !bothExist {
		return
	}

	base, res := lookup(l.baselineRun), lookup(l.resultRun)
	isPrompt := l.changeType == changeTypePrompt

	// run-config: what must not vary between the two runs.
	for _, f := range []struct {
		name     string
		b, r     string
		inPrompt bool // compared only for prompt iterations
	}{
		{"agent_model", base.agentModel, res.agentModel, false},
		{"sandbox", base.sandbox, res.sandbox, false},
		{"judge_model", base.judgeModel, res.judgeModel, true},
	} {
		if f.inPrompt && !isPrompt {
			continue
		}
		if f.b != f.r {
			add(l.path, ruleRunConfig, "%s differs between the runs: %q recorded %s, %q recorded %s", f.name, l.baselineRun, orNotRecorded(f.b), l.resultRun, orNotRecorded(f.r))
		}
	}

	checkPhase(l, base, res, add)

	havePrompt := base.promptSHA != "" && res.promptSHA != ""
	haveSuite := base.suiteSHA != "" && res.suiteSHA != ""
	samePrompt := base.promptSHA == res.promptSHA
	sameSuite := base.suiteSHA == res.suiteSHA

	if isPrompt {
		if havePrompt && samePrompt {
			add(l.path, rulePromptChange, "change_type is %q but runs %q and %q record the same prompt_sha256 (%s)", changeTypePrompt, l.baselineRun, l.resultRun, base.promptSHA)
		}
		if haveSuite && !sameSuite {
			add(l.path, ruleSuiteChange, "change_type is %q but runs %q and %q record different suite_sha256 (%s vs %s); the eval suite must stay the same", changeTypePrompt, l.baselineRun, l.resultRun, base.suiteSHA, res.suiteSHA)
		}
		return
	}

	if havePrompt && !samePrompt {
		add(l.path, rulePromptChange, "change_type is %q but runs %q and %q record different prompt_sha256 (%s vs %s); the prompt must stay the same", changeTypeEval, l.baselineRun, l.resultRun, base.promptSHA, res.promptSHA)
	}
	if haveSuite && sameSuite && base.judgeModel == res.judgeModel {
		add(l.path, ruleSuiteChange, "change_type is %q but runs %q and %q record the same suite_sha256 (%s) and the same judge_model (%s); the suite or the judge must change", changeTypeEval, l.baselineRun, l.resultRun, base.suiteSHA, orNotRecorded(base.judgeModel))
	}
}

// checkPhase applies the rule of the log's phase to its two existing runs. It
// does nothing for a log without a valid phase and for baseline, which needs no
// evidence. Scores are the recorded ones and are compared without tolerance; a
// run with no recorded score skips the comparison (run-data reports it).
func checkPhase(l *parsedLog, base, res runRecord, add func(file, rule, format string, args ...any)) {
	scoreDrop := func(rule string) {
		if base.hasScore && res.hasScore && res.score < base.score-scoreEpsilon {
			add(l.path, rule, "result run %q recorded %.2f, lower than baseline run %q which recorded %.2f; a %s iteration must not lower the score", l.resultRun, res.score, l.baselineRun, base.score, l.phase)
		}
	}
	wantChange := func(rule, want string) {
		if l.changeType != want {
			add(l.path, rule, "change_type is %q but a %s iteration must be %q", l.changeType, l.phase, want)
		}
	}

	switch l.phase {
	case phaseRed:
		wantChange(ruleRed, changeTypeEval)
		inBaseline := map[checkID]bool{}
		for _, id := range base.checks.failing() {
			inBaseline[id] = true
		}
		resFailing := res.checks.failing()
		for _, id := range resFailing {
			if !inBaseline[id] {
				return
			}
		}
		add(l.path, ruleRed, "result run %q has no failing check that baseline run %q does not also have (it records %d failing check(s), all already failing in the baseline); a red iteration needs a new failing check", l.resultRun, l.baselineRun, len(resFailing))

	case phaseGreen:
		wantChange(ruleGreen, changeTypePrompt)
		fixed := false
		for _, id := range base.checks.failing() {
			if res.checks.passesEverywhere(id) {
				fixed = true
				break
			}
		}
		if !fixed {
			add(l.path, ruleGreen, "no check that failed in baseline run %q passes in every repeat of result run %q (baseline records %d failing check(s)); a green iteration must fix one", l.baselineRun, l.resultRun, len(base.checks.failing()))
		}
		scoreDrop(ruleGreen)

	case phaseRefactor:
		wantChange(ruleRefactor, changeTypePrompt)
		scoreDrop(ruleRefactor)
	}
}

// checkCycle requires a complete Red -> Green -> Refactor cycle in the set: a
// red iteration followed later (by iteration number) by a green iteration, and
// at least one refactor iteration anywhere. Each missing part is its own
// violation, reported against the agent directory.
//
// It is label-based: it reads only the phase values of the logs that have a
// valid phase, in file-number order, and does not require those logs to pass
// their own rules (a failing log is already a violation of its own).
func checkCycle(agentDir string, logs []*parsedLog, add func(file, rule, format string, args ...any)) {
	seenRed, redThenGreen, refactor := false, false, false
	for _, l := range logs { // logs are sorted by file number
		switch l.phase {
		case phaseRed:
			seenRed = true
		case phaseGreen:
			if seenRed {
				redThenGreen = true
			}
		case phaseRefactor:
			refactor = true
		}
	}
	if !redThenGreen {
		add(agentDir, ruleCycle, "no %q iteration followed later by a %q iteration; the set must contain a complete red, green, refactor cycle", phaseRed, phaseGreen)
	}
	if !refactor {
		add(agentDir, ruleCycle, "no %q iteration; the set must contain a complete red, green, refactor cycle", phaseRefactor)
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

	rec.checks = loadCheckEvidence(result)

	// Every other fact: <agent>.json first, then summary.json, field by field.
	prov := summary.Agents[agent]
	rec.promptSHA = firstNonEmpty(result.PromptSHA256, prov.PromptSHA256)
	rec.agentModel = firstNonEmpty(result.AgentModel, prov.AgentModel)
	rec.judgeModel = firstNonEmpty(result.JudgeModel, summary.JudgeModel)
	rec.suiteSHA = firstNonEmpty(result.SuiteSHA256, prov.SuiteSHA256)
	switch {
	case result.Sandbox != nil && *result.Sandbox:
		rec.sandbox = "container"
	case result.Sandbox != nil:
		rec.sandbox = "native"
	default:
		rec.sandbox = summary.Sandbox
	}
	switch {
	case result.Complete != nil:
		rec.complete, rec.hasComplete = *result.Complete, true
	case prov.Complete != nil:
		rec.complete, rec.hasComplete = *prov.Complete, true
	}
	switch {
	case result.CaseFilter != nil:
		rec.caseFilter = *result.CaseFilter
	case prov.CaseFilter != nil:
		rec.caseFilter = *prov.CaseFilter
	}

	// Attempt success rate, from the data already loaded above: the summary
	// verdict's value when it counted attempts, else recomputed from the
	// agent result when it holds case results, else nil (not recorded).
	switch {
	case sumErr == nil && summary.AgentVerdicts[agent].AttemptsTotal > 0:
		rate := summary.AgentVerdicts[agent].SuccessRate
		rec.successRate = &rate
	case resErr == nil && len(result.Cases) > 0:
		rate := agentVerdict(result).SuccessRate
		rec.successRate = &rate
	}
	return rec
}

// firstNonEmpty returns the first argument that is not "".
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// hasVerdict reports whether the summary records a *measured* verdict for
// agent. A present-but-empty verdict object ({}) has no "score" field, and a
// present-but-null "score" decodes the numeric Score field to 0.0; neither is
// a real 0.0 score, so both are ignored and the next score source is tried. A
// measured verdict always serializes a non-null "score" field (even a real
// 0.0 as "score":0), so the raw bytes are re-read to tell these apart from a
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
	score, hasScore := fields["score"]
	return hasScore && string(score) != "null"
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
