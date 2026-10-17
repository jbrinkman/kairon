package inference

import (
	"context"
	"testing"
)

// The stub judge mirrors kiro-cli: it reports the pinned request model, and
// "stub" only when the request is unpinned.
func TestStub_JudgeModel(t *testing.T) {
	const pinned = "claude-sonnet-5.5"
	script := func() *StubScript {
		return &StubScript{Judge: StubJudge{"yes"}, Routes: []string{"fact"}}
	}
	paths := map[string]func(model string) Request{
		"default": func(m string) Request {
			return Request{Role: RoleJudge, Prompt: "p", Model: m}
		},
		"yes/no": func(m string) Request {
			return Request{Role: RoleJudge, Prompt: "p", Model: m, Stub: script(), YesNo: true}
		},
		"route": func(m string) Request {
			return Request{Role: RoleJudge, Prompt: "p", Model: m, Stub: script(), Route: true}
		},
	}
	for name, mk := range paths {
		t.Run(name, func(t *testing.T) {
			b := newTestStub(t)
			resp, err := b.Invoke(context.Background(), mk(pinned))
			if err != nil {
				t.Fatal(err)
			}
			if resp.Model != pinned {
				t.Errorf("pinned: Model = %q, want %q", resp.Model, pinned)
			}
			resp, err = b.Invoke(context.Background(), mk(""))
			if err != nil {
				t.Fatal(err)
			}
			if resp.Model != NameStub {
				t.Errorf("unpinned: Model = %q, want %q", resp.Model, NameStub)
			}
		})
	}
}
