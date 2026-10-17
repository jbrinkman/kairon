package eval

import (
	"errors"
	"fmt"

	"github.com/jbrinkman/kairon/internal/checks"
)

// The check engine lives in internal/checks so it can be reused without the
// eval harness. These aliases and wrappers keep the names the eval code and
// its tests have always used; check behavior is defined in internal/checks.

type (
	Check        = checks.Check
	CheckType    = checks.CheckType
	CheckResult  = checks.Result
	CheckInput   = checks.Input
	TurnEvidence = checks.TurnEvidence
	JudgeQuery   = checks.JudgeQuery
	JudgeVerdict = checks.JudgeVerdict
	JudgeFunc    = checks.JudgeFunc
	JudgeFile    = checks.JudgeFile
	JudgeTurn    = checks.JudgeTurn
	SimDecision  = checks.SimDecision
)

const (
	CheckCommand           = checks.CheckCommand
	CheckFileExists        = checks.CheckFileExists
	CheckFileAbsent        = checks.CheckFileAbsent
	CheckFileContains      = checks.CheckFileContains
	CheckFileNotContains   = checks.CheckFileNotContains
	CheckChangedFiles      = checks.CheckChangedFiles
	CheckOutputContains    = checks.CheckOutputContains
	CheckOutputNotContains = checks.CheckOutputNotContains
	CheckGHLogContains     = checks.CheckGHLogContains
	CheckGHLogNotContains  = checks.CheckGHLogNotContains
	CheckJudge             = checks.CheckJudge
	CheckFactAsked         = checks.CheckFactAsked
	CheckFactNotAsked      = checks.CheckFactNotAsked

	// simLogRel is the workspace-relative path of the simulated-user decision log.
	simLogRel = checks.SimLogRel
	// judgeStart and judgeEnd delimit the JSON object in a judge reply.
	judgeStart = checks.JudgeStart
	judgeEnd   = checks.JudgeEnd
	// judgeCheckTimeout bounds one judge call (judge checks and the simulated user).
	judgeCheckTimeout = checks.JudgeTimeout
)

var (
	// errInvalidChecks is wrapped by every checks validation failure.
	errInvalidChecks = checks.ErrInvalid
	// errJudgeParse marks a judge answer that could not be read as yes/no.
	errJudgeParse = checks.ErrJudgeParse
	// blockedContainerEnv are GitHub credential/host variables that never reach
	// the container, even if a caller puts them in ContainerConfig.Environment, so
	// the real gh stays unauthenticated.
	blockedContainerEnv = checks.GitHubCredentialEnv
	// errMalformedCase is wrapped by a case-file YAML decode failure so callers can
	// tell a malformed case file (fatal: the author wrote something the loader
	// could not decode, e.g. expect_exit: abc or a misspelled key) from a
	// genuinely absent/empty cases directory (skippable).
	errMalformedCase = errors.New("malformed case file")
	// errDuplicateCaseName is wrapped (alongside errMalformedCase) when two
	// case files in the same agent dir declare the same name. Coverage and
	// result files key on the case name, so a duplicate lets one case's result
	// cover another; loadCases rejects it at load time.
	errDuplicateCaseName = errors.New("duplicate case name")
)

// EvaluateChecks evaluates c against in; see checks.Evaluate.
func EvaluateChecks(c []Check, in CheckInput) []CheckResult {
	return checks.Evaluate(c, in)
}

// ValidateChecks validates checks loaded from the case file `file` and compiles
// them in place (see checks.Validate). When rubric is non-nil every check's
// criterion must be one of its non-cost criteria. Every error wraps
// errInvalidChecks and names the case file, the 1-based check index and the
// check type.
func ValidateChecks(file string, c []Check, hiddenDir string, rubric *Rubric) error {
	opts := checks.ValidateOptions{HiddenDir: hiddenDir}
	if rubric != nil {
		opts.Criterion = func(name string) error { return checkCriterion(name, rubric) }
	}
	return checks.Validate(file, c, opts)
}

// checkCriterion requires name to be a non-cost criterion of rubric.
func checkCriterion(name string, rubric *Rubric) error {
	for _, cr := range rubric.Criteria {
		if cr.Name != name {
			continue
		}
		if cr.Type == "cost" {
			return fmt.Errorf("criterion %q is a cost criterion and cannot have checks", name)
		}
		return nil
	}
	return fmt.Errorf("criterion %q is not in the rubric for agent %q", name, rubric.Agent)
}

// readSimLog reads the simulated-user decision log; see checks.ReadSimLog.
func readSimLog(dir string) ([]SimDecision, error) { return checks.ReadSimLog(dir) }

func shorten(s string, n int) string { return checks.Shorten(s, n) }

func stripANSISequences(s string) string { return checks.StripANSI(s) }
