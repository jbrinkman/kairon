package eval

import (
	"io"
	"os"

	"github.com/jbrinkman/kairon/internal/checks"
)

// debugWriter receives --debug output for judge checks. It is a variable so
// tests can capture it. The output contains file contents and agent output.
var debugWriter io.Writer = os.Stderr

// newCheckJudge returns the JudgeFunc scoreCase hands to EvaluateChecks. Each
// call invokes the configured backend as a yes/no judge on the pinned judge
// model, appends a CallRecord (with the criterion) to cr.Calls and adds the
// cost to cr.JudgeCost.
//
// Like the legacy judge, a failed or unparseable call leaves judge_cost
// untouched: the spent tokens are carried by the CallRecord in calls[] only,
// so adding them to judge_cost as well would double-count.
func newCheckJudge(tc TestCase, cr *CaseResult) JudgeFunc {
	var debug io.Writer
	if cfg.debug {
		debug = debugWriter
	}
	return checks.InferenceJudge{
		Backend: cfg.backend,
		Model:   cfg.pins.judgeModel(),
		Stub:    tc.Stub,
		Debug:   debug,
		OnCall: func(c checks.JudgeCall) {
			cost := costFromUsage(c.Response.Model, c.Response.Usage)
			rec := newCallRecord(c.Request, c.Response, c.Err, c.Wall, cost)
			rec.Criterion = c.Criterion
			cr.Calls = append(cr.Calls, rec)
			if c.Err == nil && c.ParseErr == nil {
				cr.JudgeCost.Add(cost)
			}
		},
	}.Func()
}
