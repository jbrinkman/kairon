package eval

import (
	"errors"
	"fmt"
	"io/fs"
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
	// DefaultIterationsDir is the repo-relative directory holding iteration
	// logs, laid out as <dir>/<agent>/iteration-NN.md.
	DefaultIterationsDir = ".kairon/iterations"

	// DefaultMinIterations is the number of documented iterations Stage 3 of
	// the maturity model asks for.
	DefaultMinIterations = 2
)

// Verifier rule ids. They are a stable contract: docs and later issues refer
// to them by name.
const (
	ruleFrontMatter     = "front-matter"
	ruleHeadings        = "headings"
	ruleNumbering       = "numbering"
	ruleChain           = "chain"
	ruleDateOrder       = "date-order"
	ruleRunMissing      = "run-missing"
	ruleRunNoScore      = "run-no-score"
	ruleScoreMismatch   = "score-mismatch"
	rulePromptUnchanged = "prompt-unchanged"
	rulePromptUnrecord  = "prompt-unrecorded"
	ruleMinIterations   = "min-iterations"
)

const (
	changeTypePrompt = "prompt"
	changeTypeEval   = "eval"

	// scoreTolerance is the largest allowed difference, in percentage points,
	// between a claimed score and the recorded one. scoreEpsilon absorbs
	// float error so that a difference of exactly 0.1 passes.
	scoreTolerance = 0.1
	scoreEpsilon   = 1e-9
)

var (
	iterationFileRe = regexp.MustCompile(`^iteration-(\d{2,})\.md$`)
	scoreTextRe     = regexp.MustCompile(`^(100\.0|[0-9]{1,2}\.[0-9])$`)
	unsignedIntRe   = regexp.MustCompile(`^[0-9]+$`)
	headingLineRe   = regexp.MustCompile(`^## (Baseline|Hypothesis|Change|Results|Reasoning)\s*$`)
	h2LineRe        = regexp.MustCompile(`^## `)

	requiredHeadings = []string{"Baseline", "Hypothesis", "Change", "Results", "Reasoning"}
	requiredFMKeys   = []string{"iteration", "date", "change_type", "baseline_run", "result_run", "baseline_score", "result_score"}
)

// VerifyLogOptions selects the log set to verify.
type VerifyLogOptions struct {
	Agent         string // required; a single path element
	IterationsDir string // "" => DefaultIterationsDir; logs at <dir>/<agent>/iteration-NN.md
	EvalsDir      string // "" => defaultEvalsDir; runs at <dir>/results/<run>/
	MinIterations int    // checked as given (callers pass DefaultMinIterations); must be >= 0
}

// Violation is one problem found in an iteration log set.
type Violation struct {
	File    string // path as built from IterationsDir (or the agent directory)
	Rule    string // one of the rule ids
	Message string
}

// String renders the violation as "<file>: [<rule>] <message>".
func (v Violation) String() string {
	return fmt.Sprintf("%s: [%s] %s", v.File, v.Rule, v.Message)
}

// iterFrontMatter is a successfully parsed and validated front-matter block.
type iterFrontMatter struct {
	iteration     int
	date          time.Time
	changeType    string
	baselineRun   string
	resultRun     string
	baselineScore float64
	resultScore   float64
}

// runRecord is what the verifier needs from one committed eval run.
type runRecord struct {
	loaded bool     // summary.json was read
	err    error    // why it was not
	score  *float64 // agent_scores[agent] (a fraction), nil when absent
	sha    string   // prompt_sha256 recorded for the agent, "" when absent
}

// VerifyLog reads the agent's log set and checks it against the committed
// results. It returns every violation found (nil when valid), sorted by file
// then rule, and an error only for usage/IO problems. It never touches the
// package-global cfg, so it can run without configure.
func VerifyLog(opts VerifyLogOptions) ([]Violation, error) {
	if !isSinglePathElement(opts.Agent) {
		return nil, fmt.Errorf("invalid agent name %q: must be a single path element", opts.Agent)
	}
	if opts.MinIterations < 0 {
		return nil, fmt.Errorf("invalid min iterations %d: must be >= 0", opts.MinIterations)
	}
	iterationsDir := opts.IterationsDir
	if iterationsDir == "" {
		iterationsDir = DefaultIterationsDir
	}
	evalsDir := opts.EvalsDir
	if evalsDir == "" {
		evalsDir = defaultEvalsDir
	}

	agentDir := filepath.Join(iterationsDir, opts.Agent)
	entries, err := os.ReadDir(agentDir)
	dirMissing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !dirMissing {
		return nil, fmt.Errorf("failed to read iterations directory: %w", err)
	}

	v := &verifier{agent: opts.Agent, evalsDir: evalsDir, runs: map[string]runRecord{}}

	type candidate struct {
		name string
		num  int
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "iteration-") {
			continue
		}
		m := iterationFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			v.add(filepath.Join(agentDir, e.Name()), ruleNumbering,
				"file name does not match iteration-NN.md (NN: at least two digits)")
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			v.add(filepath.Join(agentDir, e.Name()), ruleNumbering, "iteration number is out of range")
			continue
		}
		candidates = append(candidates, candidate{name: e.Name(), num: n})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].num != candidates[j].num {
			return candidates[i].num < candidates[j].num
		}
		return candidates[i].name < candidates[j].name
	})

	prevNum := -1
	var prev *iterFrontMatter // previous iteration in sequence, nil if it was invalid
	for _, c := range candidates {
		file := filepath.Join(agentDir, c.name)
		if c.num != prevNum+1 {
			v.add(file, ruleNumbering, fmt.Sprintf("iteration numbers must be contiguous from 00: expected %02d, found %02d", prevNum+1, c.num))
		}
		prevNum = c.num

		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}
		fm := v.checkFile(file, c.num, string(data), prev)
		prev = fm
	}

	if len(candidates) < opts.MinIterations {
		msg := fmt.Sprintf("found %d iteration(s), need at least %d", len(candidates), opts.MinIterations)
		if dirMissing {
			msg = fmt.Sprintf("iterations directory not found; need at least %d iteration(s)", opts.MinIterations)
		}
		v.add(agentDir, ruleMinIterations, msg)
	}

	if len(v.violations) == 0 {
		return nil, nil
	}
	sort.SliceStable(v.violations, func(i, j int) bool {
		a, b := v.violations[i], v.violations[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Rule < b.Rule
	})
	return v.violations, nil
}

type verifier struct {
	agent      string
	evalsDir   string
	runs       map[string]runRecord
	violations []Violation
}

func (v *verifier) add(file, rule, msg string) {
	v.violations = append(v.violations, Violation{File: file, Rule: rule, Message: msg})
}

// checkFile validates one iteration file and returns its front-matter, or nil
// when the front-matter was invalid.
func (v *verifier) checkFile(file string, num int, content string, prev *iterFrontMatter) *iterFrontMatter {
	fmText, body, err := splitFrontMatter(content)
	if err != nil {
		v.add(file, ruleFrontMatter, err.Error())
		return nil
	}

	for _, msg := range checkHeadings(body) {
		v.add(file, ruleHeadings, msg)
	}

	fm, problems := parseFrontMatter(fmText)
	if len(problems) > 0 {
		v.add(file, ruleFrontMatter, strings.Join(problems, "; "))
		return nil
	}

	if fm.iteration != num {
		v.add(file, ruleNumbering, fmt.Sprintf("iteration: %d does not match file name number %02d", fm.iteration, num))
	}
	if prev != nil {
		if fm.baselineRun != prev.resultRun {
			v.add(file, ruleChain, fmt.Sprintf("baseline_run %q is not the previous iteration's result_run %q", fm.baselineRun, prev.resultRun))
		}
		if fm.date.Before(prev.date) {
			v.add(file, ruleDateOrder, fmt.Sprintf("date %s is earlier than the previous iteration's %s",
				fm.date.Format("2006-01-02"), prev.date.Format("2006-01-02")))
		}
	}

	v.checkRuns(file, fm)
	return fm
}

// checkRuns applies the run-dependent rules to one iteration.
func (v *verifier) checkRuns(file string, fm *iterFrontMatter) {
	base := v.run(fm.baselineRun)
	res := v.run(fm.resultRun)

	v.checkScore(file, "baseline", fm.baselineRun, base, fm.baselineScore)
	v.checkScore(file, "result", fm.resultRun, res, fm.resultScore)

	// The prompt rules need both summaries; a missing run is already reported.
	if fm.changeType != changeTypePrompt || !base.loaded || !res.loaded {
		return
	}
	switch {
	case base.sha == "" || res.sha == "":
		var missing []string
		if base.sha == "" {
			missing = append(missing, fmt.Sprintf("baseline_run %q", fm.baselineRun))
		}
		if res.sha == "" {
			missing = append(missing, fmt.Sprintf("result_run %q", fm.resultRun))
		}
		v.add(file, rulePromptUnrecord, fmt.Sprintf("no prompt_sha256 recorded for agent %q in %s; cannot check that the prompt changed",
			v.agent, strings.Join(missing, " and ")))
	case base.sha == res.sha:
		v.add(file, rulePromptUnchanged, fmt.Sprintf("change_type is prompt but both runs record prompt_sha256 %s for agent %q",
			base.sha, v.agent))
	}
}

// checkScore reports run-missing, run-no-score or score-mismatch for one side
// (baseline or result) of an iteration.
func (v *verifier) checkScore(file, side, runName string, rec runRecord, claimed float64) {
	if !rec.loaded {
		v.add(file, ruleRunMissing, fmt.Sprintf("%s_run %q has no readable %s: %v",
			side, runName, filepath.Join(v.evalsDir, "results", runName, "summary.json"), rec.err))
		return
	}
	if rec.score == nil {
		v.add(file, ruleRunNoScore, fmt.Sprintf("%s_run %q has no agent_scores entry for agent %q", side, runName, v.agent))
		return
	}
	recorded := *rec.score * 100
	if math.Abs(claimed-recorded) > scoreTolerance+scoreEpsilon {
		v.add(file, ruleScoreMismatch, fmt.Sprintf("%s_score %.1f differs from recorded %.1f (agent_scores.%s x 100 in run %q) by more than %.1f",
			side, claimed, recorded, v.agent, runName, scoreTolerance))
	}
}

// run loads (once) what the verifier needs from a run directory. It reuses
// loadSummary and loadRunInfo (which in turn reads the agent files via
// loadAgentResult) instead of re-parsing results.
func (v *verifier) run(name string) runRecord {
	if rec, ok := v.runs[name]; ok {
		return rec
	}
	dir := filepath.Join(v.evalsDir, "results", name)
	summary, err := loadSummary(filepath.Join(dir, "summary.json"))
	rec := runRecord{}
	if err != nil {
		rec.err = err
	} else {
		rec.loaded = true
		if s, ok := summary.AgentScores[v.agent]; ok {
			s := s
			rec.score = &s
		}
		rec.sha = loadRunInfo(summary, dir).agents[v.agent].sha
	}
	v.runs[name] = rec
	return rec
}

// isSinglePathElement reports whether name is a usable single directory or
// file name: non-empty, not "." or "..", and free of separators and NULs.
func isSinglePathElement(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, "/\\\x00")
}

// splitFrontMatter splits a file into its YAML front-matter and the body. The
// first line must be exactly "---" and the block ends at the next line that is
// exactly "---".
func splitFrontMatter(content string) (frontMatter, body string, err error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return "", "", errors.New("missing front-matter: the first line must be ---")
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", "", errors.New("unterminated front-matter: no closing --- line")
}

// parseFrontMatter validates the YAML block. It returns every problem found so
// an iteration is reported once under front-matter.
func parseFrontMatter(text string) (*iterFrontMatter, []string) {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(text), &raw); err != nil {
		return nil, []string{"invalid YAML: " + err.Error()}
	}

	var problems []string
	for _, key := range requiredFMKeys {
		if _, ok := raw[key]; !ok {
			problems = append(problems, fmt.Sprintf("missing required key %q", key))
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}

	fm := &iterFrontMatter{}
	bad := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if n := raw["iteration"]; n.Kind != yaml.ScalarNode || !unsignedIntRe.MatchString(n.Value) {
		bad("iteration must be a non-negative integer")
	} else if v, err := strconv.Atoi(n.Value); err != nil {
		bad("iteration %q is out of range", n.Value)
	} else {
		fm.iteration = v
	}

	if n := raw["date"]; n.Kind != yaml.ScalarNode {
		bad("date must be YYYY-MM-DD")
	} else if d, err := time.Parse("2006-01-02", n.Value); err != nil {
		bad("date %q must be YYYY-MM-DD", n.Value)
	} else {
		fm.date = d
	}

	if n := raw["change_type"]; n.Kind != yaml.ScalarNode || (n.Value != changeTypePrompt && n.Value != changeTypeEval) {
		bad("change_type must be %q or %q", changeTypePrompt, changeTypeEval)
	} else {
		fm.changeType = n.Value
	}

	for _, f := range []struct {
		key string
		dst *string
	}{{"baseline_run", &fm.baselineRun}, {"result_run", &fm.resultRun}} {
		n := raw[f.key]
		if n.Kind != yaml.ScalarNode || !isSinglePathElement(n.Value) {
			bad("%s must be a single path element (an exact run directory name)", f.key)
			continue
		}
		*f.dst = n.Value
	}

	for _, f := range []struct {
		key string
		dst *float64
	}{{"baseline_score", &fm.baselineScore}, {"result_score", &fm.resultScore}} {
		n := raw[f.key]
		if n.Kind != yaml.ScalarNode || n.Tag != "!!float" || !scoreTextRe.MatchString(n.Value) {
			bad("%s must be a percent written with exactly one decimal between 0.0 and 100.0 (for example 87.5), found %q", f.key, n.Value)
			continue
		}
		s, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			bad("%s %q is not a number", f.key, n.Value)
			continue
		}
		*f.dst = s
	}

	if len(problems) > 0 {
		return nil, problems
	}
	return fm, nil
}

// checkHeadings returns one message per problem with the five required level-2
// headings: missing, duplicated, out of order, or without content. Headings
// inside fenced code blocks are ignored.
func checkHeadings(body string) []string {
	type section struct {
		name    string
		content strings.Builder
	}
	var sections []*section
	var cur *section
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		} else if !inFence {
			if m := headingLineRe.FindStringSubmatch(line); m != nil {
				cur = &section{name: m[1]}
				sections = append(sections, cur)
				continue
			}
			if h2LineRe.MatchString(line) {
				cur = nil // some other level-2 heading ends the section
				continue
			}
		}
		if cur != nil {
			cur.content.WriteString(line)
			cur.content.WriteString("\n")
		}
	}

	var msgs []string
	count := map[string]int{}
	var order []string // first occurrence of each heading, in file order
	for _, s := range sections {
		if count[s.name] == 0 {
			order = append(order, s.name)
		}
		count[s.name]++
		if strings.TrimSpace(s.content.String()) == "" {
			msgs = append(msgs, fmt.Sprintf("heading %q has no content", "## "+s.name))
		}
	}
	for _, h := range requiredHeadings {
		switch c := count[h]; {
		case c == 0:
			msgs = append(msgs, fmt.Sprintf("missing heading %q", "## "+h))
		case c > 1:
			msgs = append(msgs, fmt.Sprintf("heading %q appears %d times; it must appear exactly once", "## "+h, c))
		}
	}
	var want []string
	for _, h := range requiredHeadings {
		if count[h] > 0 {
			want = append(want, h)
		}
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		msgs = append(msgs, fmt.Sprintf("headings are out of order: found %s, expected %s",
			strings.Join(order, ", "), strings.Join(want, ", ")))
	}
	return msgs
}
