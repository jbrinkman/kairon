package planner

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// A Boundary with no Store, Router or Issues proves Check never reaches them.
func checkOnly() *Boundary { return &Boundary{} }

func TestBoundaryCheckAcceptsValidDocumentWithoutWriting(t *testing.T) {
	reply := interactiveCanonical(t, canonicalBody("Add retry to export", "none"))
	res, err := checkOnly().Check(interactive(reply))
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if res.Outcome != OutcomeChecked {
		t.Fatalf("outcome = %v (failure %+v), want checked", res.Outcome, res.Failure)
	}
	if len(res.Committed) != 0 {
		t.Errorf("Check committed %+v", res.Committed)
	}
}

func TestBoundaryCheckRejectsWhatHandleRejects(t *testing.T) {
	cases := map[string]Request{
		"prose":       interactive("here is your issue"),
		"no replies":  interactive(),
		"punch out":   interactive(punchOutDoc(t, stageoutput.Source{Kind: stageoutput.SourceDocument, Ref: "interactive-request"}, []stageoutput.Unresolved{{Slot: "scope", Reason: "unknown"}})),
		"bad request": {Mode: ModeAutomated, Replies: []string{"x"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := checkOnly().Check(req)
			if name == "bad request" {
				if err == nil {
					t.Fatal("Check() error = nil, want a malformed-request error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Check() error: %v", err)
			}
			assertRejected(t, res, "")
		})
	}
}

func TestBoundaryCheckResolvesPlaceholdersAgainstKnown(t *testing.T) {
	second := interactiveCanonical(t, canonicalBody("Use the store", "#{issue-1}"))

	res, err := checkOnly().Check(interactive(second))
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)

	req := interactive(second)
	req.Known = map[string]int{"issue-1": 412}
	res, err = checkOnly().Check(req)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if res.Outcome != OutcomeChecked {
		t.Fatalf("outcome = %v (failure %+v), want checked", res.Outcome, res.Failure)
	}
}

func TestBoundaryCheckReportsIndexOfRejectedReply(t *testing.T) {
	first := interactiveCanonical(t, canonicalBody("First", "none"))
	res, err := checkOnly().Check(interactive(first, "not a document"))
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	assertRejected(t, res, BoundaryGuard)
	if res.Index != 1 {
		t.Errorf("index = %d, want 1", res.Index)
	}
}
