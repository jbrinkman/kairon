package checks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrInvalid is wrapped by every checks validation failure so callers
// can tell an invalid `checks` block (fatal) from other loading errors.
var ErrInvalid = errors.New("invalid checks")

// errDuplicateCaseName is wrapped when two case files in the same agent dir
// declare the same name. Coverage (casesCovered) and result files key on the
// case name, so a duplicate name lets one case's result cover another and an
// interrupted run can be stamped complete; reject it at load time.
var errDuplicateCaseName = errors.New("duplicate case name")

// CheckType names one kind of deterministic pass/fail check.
type CheckType string

const (
	CheckCommand           CheckType = "command"
	CheckFileExists        CheckType = "file_exists"
	CheckFileAbsent        CheckType = "file_absent"
	CheckFileContains      CheckType = "file_contains"
	CheckFileNotContains   CheckType = "file_not_contains"
	CheckChangedFiles      CheckType = "changed_files"
	CheckOutputContains    CheckType = "output_contains"
	CheckOutputNotContains CheckType = "output_not_contains"
	CheckGHLogContains     CheckType = "gh_log_contains"
	CheckGHLogNotContains  CheckType = "gh_log_not_contains"
	CheckJudge             CheckType = "judge"
	CheckFactAsked         CheckType = "fact_asked"
	CheckFactNotAsked      CheckType = "fact_not_asked"

	// CheckOutputDocumentValid passes when the output parses as a stage-output
	// document with internal/stageoutput.Parse, the parser the runtime guard uses.
	CheckOutputDocumentValid CheckType = "output_document_valid"
)

// Check is one declarative pass/fail assertion, optionally attached to a rubric criterion.
type Check struct {
	Criterion  string    `yaml:"criterion" json:"criterion"`
	Type       CheckType `yaml:"type" json:"type"`
	Run        string    `yaml:"run,omitempty" json:"run,omitempty"`
	ExpectExit *int      `yaml:"expect_exit,omitempty" json:"expect_exit,omitempty"`
	Inject     []string  `yaml:"inject,omitempty" json:"inject,omitempty"`
	Path       string    `yaml:"path,omitempty" json:"path,omitempty"`
	Pattern    string    `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Allow      []string  `yaml:"allow,omitempty" json:"allow,omitempty"`
	Turn       *int      `yaml:"turn,omitempty" json:"turn,omitempty"`
	Question   string    `yaml:"question,omitempty" json:"question,omitempty"`
	Files      []string  `yaml:"files,omitempty" json:"files,omitempty"`
	Fact       string    `yaml:"fact,omitempty" json:"fact,omitempty"`

	re        *regexp.Regexp
	globs     []glob
	injectDir string
}

// Result is the outcome of one check.
type Result struct {
	Index  int       `json:"index"`
	Type   CheckType `json:"type"`
	Label  string    `json:"label"`
	Passed bool      `json:"passed"`
	Detail string    `json:"detail,omitempty"`
}

// Input is everything a check may look at.
type Input struct {
	Dir    string
	Output string
	GHLog  string
	Base   string

	// GHLogOversized is true when the gh log exceeded the read cap and GHLog
	// holds only the truncated prefix. The gh_log checks fail explicitly in
	// that case rather than scoring incomplete text.
	GHLogOversized bool

	// Turns is per-turn evidence for multi-turn cases; Turns[i] is turn i+1.
	// Empty for classic cases, whose checks read Output and GHLog.
	Turns []TurnEvidence

	// Input is the case input shown to a judge check. Optional.
	Input string

	// UserTurns[i] is the user message that produced Turns[i].Output: the
	// messages actually sent (for a simulated-user case, including the ones the
	// simulated user wrote). A judge check shows them next to the agent's
	// replies. Optional; empty for classic cases.
	UserTurns []string

	// Judge answers judge checks. Optional: a judge check evaluated with a nil
	// Judge fails ("no judge configured"); other check types ignore it.
	Judge JudgeFunc
}

// TurnEvidence is what output_* and gh_log_* checks may inspect for one turn.
type TurnEvidence struct {
	Output         string // the agent's output for the turn
	GHLog          string // .eval/gh.log as it stood at the end of the turn (cumulative)
	GHLogOversized bool
}

// expectedExit is the exit status a command check must produce (default 0).
func (c Check) expectedExit() int {
	if c.ExpectExit == nil {
		return 0
	}
	return *c.ExpectExit
}

// checkFields is the set of optional schema fields a check may carry.
type checkFields struct {
	run, expectExit, inject, path, pattern, allow, turn, question, files, fact bool
}

// checkSpecs lists, per type, the fields it requires and the fields it
// permits (required fields are permitted). Anything else is rejected, which
// catches misplaced fields and typos such as a path on a command check.
var checkSpecs = map[CheckType]struct{ required, allowed checkFields }{
	CheckCommand: {
		required: checkFields{run: true},
		allowed:  checkFields{run: true, expectExit: true, inject: true},
	},
	CheckFileExists:        {checkFields{path: true}, checkFields{path: true}},
	CheckFileAbsent:        {checkFields{path: true}, checkFields{path: true}},
	CheckFileContains:      {checkFields{path: true, pattern: true}, checkFields{path: true, pattern: true}},
	CheckFileNotContains:   {checkFields{path: true, pattern: true}, checkFields{path: true, pattern: true}},
	CheckChangedFiles:      {checkFields{allow: true}, checkFields{allow: true}},
	CheckOutputContains:    {checkFields{pattern: true}, checkFields{pattern: true, turn: true}},
	CheckOutputNotContains: {checkFields{pattern: true}, checkFields{pattern: true, turn: true}},
	CheckGHLogContains:     {checkFields{pattern: true}, checkFields{pattern: true, turn: true}},
	CheckGHLogNotContains:  {checkFields{pattern: true}, checkFields{pattern: true, turn: true}},
	CheckJudge:             {checkFields{question: true}, checkFields{question: true, files: true, turn: true}},
	CheckFactAsked:         {checkFields{fact: true}, checkFields{fact: true}},
	CheckFactNotAsked:      {checkFields{fact: true}, checkFields{fact: true}},

	CheckOutputDocumentValid: {checkFields{}, checkFields{turn: true}},
}

// present reports which fields the check actually sets. Empty strings, a nil
// slice and a nil expect_exit count as unset; allow: [] is set.
func (c Check) present() checkFields {
	return checkFields{
		run:        strings.TrimSpace(c.Run) != "",
		expectExit: c.ExpectExit != nil,
		inject:     len(c.Inject) > 0,
		path:       c.Path != "",
		pattern:    c.Pattern != "",
		allow:      c.Allow != nil,
		turn:       c.Turn != nil,
		question:   c.Question != "",
		files:      len(c.Files) > 0,
		fact:       strings.TrimSpace(c.Fact) != "",
	}
}

func validateCheck(c *Check, opts ValidateOptions) error {
	if c.Criterion == "" && !opts.OptionalCriterion {
		return errors.New("criterion is required")
	}
	if c.Type == "" {
		return errors.New("type is required")
	}
	spec, ok := checkSpecs[c.Type]
	if !ok {
		return fmt.Errorf("unknown type %q", c.Type)
	}

	have := c.present()
	for _, f := range []struct {
		name               string
		have, required, ok bool
	}{
		{"run", have.run, spec.required.run, spec.allowed.run},
		{"expect_exit", have.expectExit, spec.required.expectExit, spec.allowed.expectExit},
		{"inject", have.inject, spec.required.inject, spec.allowed.inject},
		{"path", have.path, spec.required.path, spec.allowed.path},
		{"pattern", have.pattern, spec.required.pattern, spec.allowed.pattern},
		{"allow", have.allow, spec.required.allow, spec.allowed.allow},
		{"turn", have.turn, spec.required.turn, spec.allowed.turn},
		{"question", have.question, spec.required.question, spec.allowed.question},
		{"files", have.files, spec.required.files, spec.allowed.files},
		{"fact", have.fact, spec.required.fact, spec.allowed.fact},
	} {
		switch {
		case f.have && !f.ok:
			return fmt.Errorf("%s is not valid for type %s", f.name, c.Type)
		case !f.have && f.required:
			return fmt.Errorf("%s is required", f.name)
		}
	}

	if opts.Criterion != nil {
		if err := opts.Criterion(c.Criterion); err != nil {
			return err
		}
	}

	if have.turn && *c.Turn < 1 {
		return fmt.Errorf("turn %d must be >= 1 (turns are numbered from 1)", *c.Turn)
	}
	if have.path {
		if !filepath.IsLocal(c.Path) {
			return fmt.Errorf("path %q must be relative and stay inside the workspace (no absolute path, no '..')", c.Path)
		}
	}
	if c.Type == CheckJudge {
		if err := validateJudgeCheck(c); err != nil {
			return err
		}
	}
	if have.pattern {
		re, err := regexp.Compile(c.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern %q: %w", c.Pattern, err)
		}
		c.re = re
	}
	if have.allow {
		globs := make([]glob, 0, len(c.Allow))
		for _, a := range c.Allow {
			g, err := compileGlob(a)
			if err != nil {
				return err
			}
			globs = append(globs, g)
		}
		c.globs = globs
	}
	if c.Type == CheckCommand {
		if have.expectExit && (*c.ExpectExit < 0 || *c.ExpectExit > 255) {
			return fmt.Errorf("expect_exit %d out of range 0-255", *c.ExpectExit)
		}
		if !have.expectExit {
			zero := 0
			c.ExpectExit = &zero
		}
		if have.inject {
			dir, err := resolveInjects(c.Inject, opts.HiddenDir)
			if err != nil {
				return err
			}
			c.injectDir = dir
		}
	}
	return nil
}

// reservedInjectRoots are workspace entries an inject destination must never
// land in: git metadata, the harness's own directory and agent config.
var reservedInjectRoots = map[string]bool{".git": true, ".eval": true, ".kiro": true}

// resolveInjects validates the inject entries against hiddenDir and returns
// the absolute hidden directory. Each entry must be a local, existing,
// regular, non-symlink file whose destination avoids the reserved roots.
func resolveInjects(entries []string, hiddenDir string) (string, error) {
	if hiddenDir == "" {
		return "", errors.New("inject: no hidden fixtures directory is configured")
	}
	abs, err := filepath.Abs(hiddenDir)
	if err != nil {
		return "", fmt.Errorf("inject: %w", err)
	}
	for _, e := range entries {
		if e == "" || !filepath.IsLocal(e) {
			return "", fmt.Errorf("inject %q must be a relative path inside fixtures/hidden (no absolute path, no '..')", e)
		}
		clean := filepath.Clean(e)
		parts := strings.Split(filepath.ToSlash(clean), "/")
		if reservedInjectRoots[parts[0]] {
			return "", fmt.Errorf("inject %q: destination %q is reserved", e, parts[0])
		}
		for _, p := range parts {
			if p == ".git" {
				return "", fmt.Errorf("inject %q: destination must not be inside .git", e)
			}
		}
		// Lstat every component so neither the file nor a parent directory
		// may be a symlink.
		cur := abs
		for j, p := range parts {
			cur = filepath.Join(cur, p)
			info, err := os.Lstat(cur)
			if err != nil {
				return "", fmt.Errorf("inject %q: source not found under %s: %w", e, abs, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("inject %q: source %q is a symlink, which is not allowed", e, filepath.Join(parts[:j+1]...))
			}
			if j == len(parts)-1 && !info.Mode().IsRegular() {
				return "", fmt.Errorf("inject %q: source must be a regular file", e)
			}
		}
	}
	return abs, nil
}
