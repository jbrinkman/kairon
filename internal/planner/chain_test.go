package planner

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// splitRequest is one issue of a split: a canonical reply whose Dependencies
// section is deps.
func splitRequest(t *testing.T, key, deps string) ChainRequest {
	t.Helper()
	return ChainRequest{Key: key, Reply: canonicalReply(t, validBody(deps))}
}

// storedNumbers returns the issue numbers the fake store holds, ascending.
func storedNumbers(s *fakeStore) []int {
	var nums []int
	for n := range s.bodies {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	return nums
}

func TestProcessChainResolvesDependenciesIntoSingleLineForm(t *testing.T) {
	t.Parallel()
	h := newHarness()

	res, err := h.b.ProcessChain(context.Background(), []ChainRequest{
		splitRequest(t, "a", "Dependencies: none\n"),
		splitRequest(t, "b", "Dependencies: #{a}\n"),
		splitRequest(t, "c", "Dependencies: #{a}, #{b}\n"),
	})
	if err != nil {
		t.Fatalf("ProcessChain: %v", err)
	}
	if res.Stopped != nil || res.StoppedAt != -1 {
		t.Fatalf("chain stopped at %d: %+v", res.StoppedAt, res.Stopped)
	}
	if got := storedNumbers(h.store); !slices.Equal(got, []int{101, 102, 103}) {
		t.Fatalf("committed issues = %v, want [101 102 103] in order", got)
	}
	if len(res.Committed) != 3 {
		t.Fatalf("Committed has %d entries, want 3", len(res.Committed))
	}
	for i, key := range []string{"a", "b", "c"} {
		if res.Committed[i].Key != key || res.Committed[i].Result.Number != 101+i {
			t.Errorf("Committed[%d] = %q #%d, want %q #%d", i, res.Committed[i].Key, res.Committed[i].Result.Number, key, 101+i)
		}
	}

	for number, want := range map[int]string{
		101: "Dependencies: none",
		102: "Dependencies: #101",
		103: "Dependencies: #101, #102",
	} {
		body := h.store.bodies[number]
		if !hasLine(body, want) {
			t.Errorf("issue %d lacks the line %q:\n%s", number, want, body)
		}
		if strings.Contains(body, "#{") {
			t.Errorf("issue %d still carries a placeholder:\n%s", number, body)
		}
	}
}

func TestProcessChainCommittedBodiesHaveNoBulletListAndValidHash(t *testing.T) {
	t.Parallel()
	h := newHarness()

	res, err := h.b.ProcessChain(context.Background(), []ChainRequest{
		splitRequest(t, "a", "Dependencies: none\n"),
		splitRequest(t, "b", "Dependencies: #{a}\n"),
		splitRequest(t, "c", "Dependencies: #{a}, #{b}\n"),
	})
	if err != nil || res.Stopped != nil {
		t.Fatalf("ProcessChain = %+v, %v", res, err)
	}
	for number, body := range h.store.bodies {
		if bulletUnderDependencies(body) {
			t.Errorf("issue %d has a bullet list under ## Dependencies:\n%s", number, body)
		}
		if state := provenance.Classify(body); state != provenance.StateValid {
			t.Errorf("issue %d classifies as %v, want valid", number, state)
		}
	}
	for _, c := range res.Committed {
		if c.Result.Hash == "" || !strings.Contains(h.store.bodies[c.Result.Number], "sha256="+c.Result.Hash) {
			t.Errorf("issue %d: stored marker does not carry the reported hash %q", c.Result.Number, c.Result.Hash)
		}
	}
}

func TestProcessChainGuardFailureStopsChainAndKeepsEarlierDocuments(t *testing.T) {
	t.Parallel()
	h := newHarness()
	// The second document has no Verify: line, so the structural guard rejects it.
	broken := strings.Replace(validBody("Dependencies: #{a}\n"),
		"   Verify: run `kairon export --format jsonl`; every line parses with `jq -e .`.\n", "", 1)

	res, err := h.b.ProcessChain(context.Background(), []ChainRequest{
		splitRequest(t, "a", "Dependencies: none\n"),
		{Key: "b", Reply: canonicalReply(t, broken)},
		splitRequest(t, "c", "Dependencies: #{a}, #{b}\n"),
	})
	if err != nil {
		t.Fatalf("ProcessChain: %v", err)
	}
	if res.StoppedAt != 1 || res.Stopped == nil {
		t.Fatalf("StoppedAt = %d, Stopped = %+v, want a stop at item 1", res.StoppedAt, res.Stopped)
	}
	if res.Stopped.Admission.Outcome != guard.OutcomeRejected || res.Stopped.Admission.Failure == nil {
		t.Fatalf("admission = %+v, want a rejection with a failure context", res.Stopped.Admission)
	}
	if got := res.Stopped.Admission.Failure.Guard; got != guard.StructuralName {
		t.Errorf("failing guard = %q, want %q", got, guard.StructuralName)
	}
	if len(res.Committed) != 1 || res.Committed[0].Key != "a" {
		t.Errorf("Committed = %+v, want only item a", res.Committed)
	}
	if got := storedNumbers(h.store); !slices.Equal(got, []int{101}) {
		t.Errorf("store holds %v, want only issue 101 (the third document must not be attempted)", got)
	}
	if provenance.Classify(h.store.bodies[101]) != provenance.StateValid {
		t.Errorf("the earlier document is not left committed with a valid marker")
	}
}

func TestProcessChainForwardReferenceStopsWithUnknownPlaceholder(t *testing.T) {
	t.Parallel()
	h := newHarness()

	res, err := h.b.ProcessChain(context.Background(), []ChainRequest{
		splitRequest(t, "a", "Dependencies: none\n"),
		splitRequest(t, "b", "Dependencies: #{c}\n"), // c is committed after b
		splitRequest(t, "c", "Dependencies: #{a}\n"),
	})
	if !errors.Is(err, provenance.ErrUnknownPlaceholder) {
		t.Fatalf("error = %v, want provenance.ErrUnknownPlaceholder", err)
	}
	if res.StoppedAt != 1 {
		t.Errorf("StoppedAt = %d, want 1", res.StoppedAt)
	}
	if got := storedNumbers(h.store); !slices.Equal(got, []int{101}) {
		t.Errorf("store holds %v, want only issue 101", got)
	}
}

func TestProcessChainMalformedReplyWritesNothing(t *testing.T) {
	t.Parallel()
	cases := map[string][]ChainRequest{
		"reply is not a document": {{Key: "a", Reply: "not a document"}},
		"later reply is empty":    nil, // filled in below
		"punch_out in a chain":    nil, // filled in below
	}
	cases["later reply is empty"] = []ChainRequest{splitRequest(t, "a", "Dependencies: none\n"), {Key: "b", Reply: ""}}
	cases["punch_out in a chain"] = []ChainRequest{splitRequest(t, "a", "Dependencies: none\n"), {Key: "b", Reply: punchOutReply(t)}}

	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness()
			_, err := h.b.ProcessChain(context.Background(), items)
			if !errors.Is(err, guard.ErrInvalid) {
				t.Fatalf("error = %v, want guard.ErrInvalid", err)
			}
			// Every reply is read before the first write, so a bad later
			// reply cannot leave earlier issues behind.
			h.assertNothingWritten(t)
		})
	}
}

func TestProcessChainRequiresCommitterAndItems(t *testing.T) {
	t.Parallel()
	h := newHarness()
	if _, err := h.b.ProcessChain(context.Background(), nil); !errors.Is(err, provenance.ErrInvalid) {
		t.Errorf("empty chain error = %v, want provenance.ErrInvalid", err)
	}
	h.b.Committer = nil
	_, err := h.b.ProcessChain(context.Background(), []ChainRequest{splitRequest(t, "a", "Dependencies: none\n")})
	if !errors.Is(err, provenance.ErrInvalid) {
		t.Errorf("no-committer error = %v, want provenance.ErrInvalid", err)
	}
}

func TestProcessChainHonoursTitleAndInjectedGuards(t *testing.T) {
	t.Parallel()
	h := newHarness()
	req := splitRequest(t, "a", "Dependencies: none\n")
	req.Title = "Explicit title"
	if _, err := h.b.ProcessChain(context.Background(), []ChainRequest{req}); err != nil {
		t.Fatalf("ProcessChain: %v", err)
	}
	if got := h.store.titles[101]; got != "Explicit title" {
		t.Errorf("title = %q, want the request title", got)
	}

	h = newHarness()
	h.b.Guards = func(handler guard.FailureHandler) []guard.Guard {
		return []guard.Guard{rejectAll{handler: handler}}
	}
	res, err := h.b.ProcessChain(context.Background(), []ChainRequest{splitRequest(t, "a", "Dependencies: none\n")})
	if err != nil {
		t.Fatalf("ProcessChain: %v", err)
	}
	if res.Stopped == nil || res.Stopped.Admission.Failure == nil || res.Stopped.Admission.Failure.Guard != "reject-all" {
		t.Fatalf("result = %+v, want a stop by the injected guard", res)
	}
	h.assertNothingWritten(t)
}

// hasLine reports whether body has a line equal to want.
func hasLine(body, want string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// bulletUnderDependencies reports whether a list item appears between a
// "## Dependencies" heading and the next heading.
func bulletUnderDependencies(body string) bool {
	in := false
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
			in = strings.EqualFold(strings.TrimSpace(strings.TrimLeft(line, "#")), "dependencies")
		case in && depsBulletRE.MatchString(line):
			return true
		}
	}
	return false
}
