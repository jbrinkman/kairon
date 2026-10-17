package checks

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// JudgeTimeout bounds one judge-check call.
const JudgeTimeout = 2 * time.Minute

// JudgeCall describes one judge model call, for the caller's accounting.
type JudgeCall struct {
	Criterion string
	Request   inference.Request
	Response  inference.Response
	Err       error // the Invoke error, if any
	ParseErr  error // set when the reply was not a yes/no verdict
	Wall      time.Duration
}

// InferenceJudge builds a JudgeFunc that asks a yes/no question of Backend.
type InferenceJudge struct {
	Backend inference.Backend
	Model   string                // pinned judge model ("" = unpinned)
	Stub    *inference.StubScript // only the stub backend reads it
	Debug   io.Writer             // when non-nil, each prompt is written to it
	OnCall  func(JudgeCall)       // optional; called after every Invoke
}

// Func returns the JudgeFunc. Each call invokes Backend as a yes/no judge on
// Model, reports the call through OnCall and parses the reply. A failed call
// returns the Invoke error; an unparseable reply returns an error wrapping
// ErrJudgeParse.
func (j InferenceJudge) Func() JudgeFunc {
	n := 0
	return func(q JudgeQuery) (JudgeVerdict, error) {
		n++
		prompt := BuildYesNoPrompt(q)
		if j.Debug != nil {
			fmt.Fprintf(j.Debug, "🔧 Debug: judge prompt (criterion=%s, judge check #%d)\n%s\n", q.Criterion, n, prompt)
		}
		req := inference.Request{
			Role:    inference.RoleJudge,
			Prompt:  prompt,
			Timeout: JudgeTimeout,
			Model:   j.Model,
			Stub:    j.Stub,
			YesNo:   true,
		}
		wallStart := time.Now()
		resp, err := j.Backend.Invoke(context.Background(), req)
		call := JudgeCall{Criterion: q.Criterion, Request: req, Response: resp, Err: err, Wall: time.Since(wallStart)}
		if err != nil {
			j.report(call)
			return JudgeVerdict{}, err
		}
		verdict, perr := ParseYesNo(resp.Text)
		call.ParseErr = perr
		j.report(call)
		if perr != nil {
			return JudgeVerdict{}, perr
		}
		return verdict, nil
	}
}

func (j InferenceJudge) report(c JudgeCall) {
	if j.OnCall != nil {
		j.OnCall(c)
	}
}
