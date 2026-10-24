package watcher

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/github"
)

func TestParseDependencies(t *testing.T) {
	parser := &DependencyParser{}

	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "depends on issue format",
			input:    "Depends on Issue #88",
			expected: []int{88},
		},
		{
			name:     "dependencies comma format",
			input:    "Dependencies: #88, #89",
			expected: []int{88, 89},
		},
		{
			name:     "blocked by format",
			input:    "Blocked by: #90",
			expected: []int{90},
		},
		{
			name:     "markdown link format",
			input:    "Depends on [Issue #88](https://github.com/user/repo/issues/88)",
			expected: []int{88},
		},
		{
			name:     "multiple formats mixed",
			input:    "Depends on Issue #88\nDependencies: #89, #90\nBlocked by: #91",
			expected: []int{88, 89, 90, 91},
		},
		{
			name:     "no dependencies",
			input:    "This is a regular issue with no dependencies",
			expected: []int{},
		},
		{
			name:     "malformed input",
			input:    "Depends on Issue # \nDependencies: #abc, #",
			expected: []int{},
		},
		{
			name:     "duplicate issue numbers",
			input:    "Depends on Issue #88\nDependencies: #88, #89",
			expected: []int{88, 89},
		},
		{
			name:     "case insensitive",
			input:    "depends on issue #88\nDEPENDENCIES: #89\nblocked by: #90",
			expected: []int{88, 89, 90},
		},
		{
			name:     "dependencies without hash",
			input:    "Dependencies: 88, 89",
			expected: []int{88, 89},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.ParseDependencies(tt.input)

			// Sort both slices for comparison since order doesn't matter
			sort.Ints(result)
			sort.Ints(tt.expected)

			// Handle nil vs empty slice comparison
			if len(result) != len(tt.expected) {
				t.Errorf("ParseDependencies() = %v, expected %v", result, tt.expected)
			} else if len(result) > 0 && !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseDependencies() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// fenceBody joins lines with "\n". Test bodies are built from lines so the
// fence characters appear as ordinary Go string content.
func fenceBody(lines ...string) string {
	return strings.Join(lines, "\n")
}

func TestParseDependenciesFencedCode(t *testing.T) {
	parser := &DependencyParser{}

	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name: "AC1 none outside, fenced dependency ignored",
			input: fenceBody(
				"Dependencies: none",
				"",
				"```",
				"Dependencies: #397",
				"```",
			),
			expected: nil,
		},
		{
			name:     "AC2 unfenced comma list",
			input:    fenceBody("Dependencies: #380, #379"),
			expected: []int{380, 379},
		},
		{
			name:     "AC3a fenced depends on issue ignored",
			input:    fenceBody("```", "depends on issue #397", "```"),
			expected: nil,
		},
		{
			name:     "AC3b fenced blocked by ignored",
			input:    fenceBody("```", "blocked by: #392", "```"),
			expected: nil,
		},
		{
			name:     "AC3c same declarations unfenced still parsed",
			input:    fenceBody("depends on issue #397", "", "blocked by: #392"),
			expected: []int{397, 392},
		},
		{
			name:     "AC3d both declarations fenced ignored",
			input:    fenceBody("```", "depends on issue #397", "blocked by: #392", "```"),
			expected: nil,
		},
		{
			name: "mixed real dependency before fenced one",
			input: fenceBody(
				"Depends on Issue #5",
				"",
				"```",
				"Dependencies: #397",
				"```",
			),
			expected: []int{5},
		},
		{
			name: "mixed fenced then real dependency after",
			input: fenceBody(
				"```",
				"Dependencies: #397",
				"```",
				"",
				"Depends on Issue #5",
			),
			expected: []int{5},
		},
		{
			name:     "tilde fence",
			input:    fenceBody("~~~", "Dependencies: #397", "~~~"),
			expected: nil,
		},
		{
			name:     "info string on opener",
			input:    fenceBody("```markdown", "Dependencies: #397", "```"),
			expected: nil,
		},
		{
			name:     "fence indented one space",
			input:    fenceBody(" ```", " Dependencies: #397", " ```"),
			expected: nil,
		},
		{
			name:     "fence indented three spaces",
			input:    fenceBody("   ```", "   Dependencies: #397", "   ```"),
			expected: nil,
		},
		{
			name:     "four-space indent is not a fence",
			input:    fenceBody("    ```", "Dependencies: #397", "    ```"),
			expected: []int{397},
		},
		{
			name: "longer opener not closed by shorter run",
			input: fenceBody(
				"````",
				"```",
				"Dependencies: #397",
				"````",
			),
			expected: nil,
		},
		{
			name: "longer closer closes",
			input: fenceBody(
				"```",
				"Dependencies: #397",
				"`````",
				"",
				"Depends on Issue #6",
			),
			expected: []int{6},
		},
		{
			name: "mismatched char does not close",
			input: fenceBody(
				"```",
				"~~~",
				"Dependencies: #397",
			),
			expected: nil,
		},
		{
			name: "closer with trailing text does not close",
			input: fenceBody(
				"```",
				"``` not a closer",
				"Dependencies: #397",
			),
			expected: nil,
		},
		{
			name: "closer with trailing whitespace closes",
			input: fenceBody(
				"```",
				"Dependencies: #397",
				"```   ",
				"Depends on Issue #7",
			),
			expected: []int{7},
		},
		{
			name: "unclosed fence runs to end of body",
			input: fenceBody(
				"```",
				"intro",
				"Dependencies: #397",
			),
			expected: nil,
		},
		{
			name: "dependency before unclosed fence is kept",
			input: fenceBody(
				"Depends on Issue #8",
				"```",
				"Dependencies: #397",
			),
			expected: []int{8},
		},
		{
			name:     "markdown link form fenced",
			input:    fenceBody("```", "Depends on [Issue #88]", "```"),
			expected: nil,
		},
		{
			name:     "CRLF fenced block",
			input:    "Dependencies: none\r\n\r\n```\r\nDependencies: #397\r\n```\r\n",
			expected: nil,
		},
		{
			name:     "CRLF unfenced still parsed",
			input:    "Dependencies: #380, #379\r\n",
			expected: []int{380, 379},
		},
		{
			name: "two separate fences with real dependency between",
			input: fenceBody(
				"```",
				"Dependencies: #1",
				"```",
				"Blocked by: #2",
				"~~~",
				"Dependencies: #3",
				"~~~",
			),
			expected: []int{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.ParseDependencies(tt.input)
			if len(got) == 0 && len(tt.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ParseDependencies() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStripFencedCode(t *testing.T) {
	t.Run("no fence returns input unchanged", func(t *testing.T) {
		for _, in := range []string{
			"",
			"Dependencies: #1",
			"line one\n\nline two\r\nline three\n",
			"    ```\nindented four spaces\n",
			"inline ``` backticks mid-line",
		} {
			if got := stripFencedCode(in); got != in {
				t.Errorf("stripFencedCode(%q) = %q, want unchanged", in, got)
			}
		}
	})

	t.Run("fenced lines blanked and delimiters included", func(t *testing.T) {
		in := fenceBody("before", "```go", "x := 1", "```", "after")
		want := fenceBody("before", "", "", "", "after")
		if got := stripFencedCode(in); got != want {
			t.Errorf("stripFencedCode() = %q, want %q", got, want)
		}
	})

	t.Run("line count preserved", func(t *testing.T) {
		in := fenceBody("a", "~~~", "b", "c", "~~~", "d", "```", "e")
		got := stripFencedCode(in)
		if strings.Count(got, "\n") != strings.Count(in, "\n") {
			t.Errorf("newline count changed: got %q from %q", got, in)
		}
		if want := fenceBody("a", "", "", "", "", "d", "", ""); got != want {
			t.Errorf("stripFencedCode() = %q, want %q", got, want)
		}
	})

	t.Run("unfenced text around fence is byte-identical", func(t *testing.T) {
		in := "keep\r\n```\r\nDependencies: #9\r\n```\r\nkeep too\r\n"
		got := stripFencedCode(in)
		if strings.Contains(got, "#9") || strings.Contains(got, "```") {
			t.Errorf("fenced content survived: %q", got)
		}
		if !strings.HasPrefix(got, "keep\r\n") || !strings.HasSuffix(got, "keep too\r\n") {
			t.Errorf("unfenced text altered: %q", got)
		}
	})

	t.Run("unclosed fence blanks to end", func(t *testing.T) {
		in := fenceBody("keep", "```", "gone", "gone too")
		want := fenceBody("keep", "", "", "")
		if got := stripFencedCode(in); got != want {
			t.Errorf("stripFencedCode() = %q, want %q", got, want)
		}
	})
}

func TestBackoffTracker(t *testing.T) {
	t.Run("new issue should be checked", func(t *testing.T) {
		bt := NewBackoffTracker()
		bt.IncrementRound()
		if !bt.ShouldCheck(1) {
			t.Error("expected new issue to be checked")
		}
	})

	t.Run("backoff progression 2x 4x 8x 16x max", func(t *testing.T) {
		bt := NewBackoffTracker()
		bt.IncrementRound() // round 1

		// First failure: delay 2 rounds (next check at round 3)
		bt.RecordFailure(1)
		if bt.ShouldCheck(1) {
			t.Error("expected issue to be in backoff after first failure")
		}
		if got := bt.GetRoundsUntilCheck(1); got != 2 {
			t.Errorf("expected 2 rounds until check, got %d", got)
		}

		bt.IncrementRound() // round 2
		if bt.ShouldCheck(1) {
			t.Error("expected issue still in backoff at round 2")
		}

		bt.IncrementRound() // round 3
		if !bt.ShouldCheck(1) {
			t.Error("expected issue to be checkable at round 3")
		}

		// Second failure: delay 4 rounds (next check at round 7)
		bt.RecordFailure(1)
		if bt.ShouldCheck(1) {
			t.Error("expected issue to be in backoff after second failure")
		}
		if got := bt.GetRoundsUntilCheck(1); got != 4 {
			t.Errorf("expected 4 rounds until check, got %d", got)
		}

		// Third failure: delay 8 rounds
		for i := 0; i < 4; i++ {
			bt.IncrementRound()
		}
		bt.RecordFailure(1)
		if got := bt.GetRoundsUntilCheck(1); got != 8 {
			t.Errorf("expected 8 rounds until check, got %d", got)
		}

		// Fourth failure: delay 16 rounds (max)
		for i := 0; i < 8; i++ {
			bt.IncrementRound()
		}
		bt.RecordFailure(1)
		if got := bt.GetRoundsUntilCheck(1); got != 16 {
			t.Errorf("expected 16 rounds until check (max), got %d", got)
		}

		// Fifth failure: still capped at 16
		for i := 0; i < 16; i++ {
			bt.IncrementRound()
		}
		bt.RecordFailure(1)
		if got := bt.GetRoundsUntilCheck(1); got != 16 {
			t.Errorf("expected 16 rounds (cap), got %d", got)
		}
	})
}

// fakeIssueLookup returns a lookup function that records every requested issue
// number in *calls and answers from the scripted details/errs maps. An issue
// found in errs returns that error; otherwise it returns the entry in details.
// It never invokes the real gh CLI.
func fakeIssueLookup(calls *[]int, details map[int]*github.IssueDetails, errs map[int]error) issueLookupFunc {
	return func(_ string, number int) (*github.IssueDetails, error) {
		*calls = append(*calls, number)
		if err, ok := errs[number]; ok {
			return nil, err
		}
		if d, ok := details[number]; ok {
			return d, nil
		}
		return nil, errors.New("unexpected lookup of issue")
	}
}

func TestValidateIssue(t *testing.T) {
	lookupErr := errors.New("gh: network unreachable")
	const body = "Dependencies: #10, #11, #12"

	tests := []struct {
		name           string
		body           string
		details        map[int]*github.IssueDetails
		errs           map[int]error
		wantLookups    []int
		wantValid      bool
		wantUnresolved []int
	}{
		{
			name: "first dependency open stops after one lookup",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "open"},
				11: {State: "closed"},
				12: {State: "closed"},
			},
			wantLookups:    []int{10},
			wantValid:      false,
			wantUnresolved: []int{10},
		},
		{
			name: "later dependency open stops at that dependency",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "closed"},
				11: {State: "open"},
				12: {State: "open"},
			},
			wantLookups:    []int{10, 11},
			wantValid:      false,
			wantUnresolved: []int{11},
		},
		{
			name: "last dependency open is reported after all prior lookups",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "closed"},
				11: {State: "closed"},
				12: {State: "open"},
			},
			wantLookups:    []int{10, 11, 12},
			wantValid:      false,
			wantUnresolved: []int{12},
		},
		{
			name: "all dependencies closed",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "closed"},
				11: {State: "closed"},
				12: {State: "closed"},
			},
			wantLookups: []int{10, 11, 12},
			wantValid:   true,
		},
		{
			name: "closed state is case insensitive",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "Closed"},
				11: {State: "CLOSED"},
				12: {State: "closed"},
			},
			wantLookups: []int{10, 11, 12},
			wantValid:   true,
		},
		{
			name:           "first dependency lookup error stops the loop",
			body:           body,
			errs:           map[int]error{10: lookupErr},
			wantLookups:    []int{10},
			wantValid:      false,
			wantUnresolved: []int{10},
		},
		{
			name: "later dependency lookup error stops the loop",
			body: body,
			details: map[int]*github.IssueDetails{
				10: {State: "closed"},
			},
			errs:           map[int]error{11: lookupErr},
			wantLookups:    []int{10, 11},
			wantValid:      false,
			wantUnresolved: []int{11},
		},
		{
			name:        "no dependencies performs no lookups",
			body:        "This is a regular issue with no dependencies",
			wantLookups: nil,
			wantValid:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []int
			dv := NewDependencyValidator()
			dv.getIssueDetails = fakeIssueLookup(&calls, tt.details, tt.errs)

			result, err := dv.ValidateIssue("owner/repo", 1, tt.body)
			if err != nil {
				t.Fatalf("ValidateIssue() returned unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("ValidateIssue() returned nil result")
			}

			if len(calls) != len(tt.wantLookups) || (len(calls) > 0 && !reflect.DeepEqual(calls, tt.wantLookups)) {
				t.Errorf("lookups = %v, expected %v", calls, tt.wantLookups)
			}
			if result.IsValid != tt.wantValid {
				t.Errorf("IsValid = %v, expected %v", result.IsValid, tt.wantValid)
			}
			if len(result.UnresolvedDependencies) != len(tt.wantUnresolved) ||
				(len(tt.wantUnresolved) > 0 && !reflect.DeepEqual(result.UnresolvedDependencies, tt.wantUnresolved)) {
				t.Errorf("UnresolvedDependencies = %v, expected %v", result.UnresolvedDependencies, tt.wantUnresolved)
			}
			if len(result.CircularDependencies) != 0 {
				t.Errorf("CircularDependencies = %v, expected empty", result.CircularDependencies)
			}
		})
	}
}

func TestValidateIssueCyclicDependencyDoesNotFetchChain(t *testing.T) {
	t.Run("two issue cycle only looks up the direct dependency", func(t *testing.T) {
		// Issue 10 depends on #11, whose body points back at #10.
		var calls []int
		details := map[int]*github.IssueDetails{
			11: {State: "closed", Body: "Dependencies: #10"},
		}
		dv := NewDependencyValidator()
		dv.getIssueDetails = fakeIssueLookup(&calls, details, nil)

		result, err := dv.ValidateIssue("owner/repo", 10, "Dependencies: #11")
		if err != nil {
			t.Fatalf("ValidateIssue() returned unexpected error: %v", err)
		}

		if !reflect.DeepEqual(calls, []int{11}) {
			t.Errorf("lookups = %v, expected [11] (no recursive chain fetch)", calls)
		}
		if !result.IsValid {
			t.Errorf("IsValid = false, expected true (dependency is closed)")
		}
		if len(result.CircularDependencies) != 0 {
			t.Errorf("CircularDependencies = %v, expected empty", result.CircularDependencies)
		}
	})

	t.Run("self referencing issue is looked up once", func(t *testing.T) {
		// Issue 10 depends on itself; the dependency body repeats the cycle.
		var calls []int
		details := map[int]*github.IssueDetails{
			10: {State: "open", Body: "Dependencies: #10"},
		}
		dv := NewDependencyValidator()
		dv.getIssueDetails = fakeIssueLookup(&calls, details, nil)

		result, err := dv.ValidateIssue("owner/repo", 10, "Dependencies: #10")
		if err != nil {
			t.Fatalf("ValidateIssue() returned unexpected error: %v", err)
		}

		if !reflect.DeepEqual(calls, []int{10}) {
			t.Errorf("lookups = %v, expected [10] (no recursive chain fetch)", calls)
		}
		if result.IsValid {
			t.Errorf("IsValid = true, expected false (dependency is open)")
		}
		if !reflect.DeepEqual(result.UnresolvedDependencies, []int{10}) {
			t.Errorf("UnresolvedDependencies = %v, expected [10]", result.UnresolvedDependencies)
		}
		if len(result.CircularDependencies) != 0 {
			t.Errorf("CircularDependencies = %v, expected empty", result.CircularDependencies)
		}
	})
}

// fencedBody builds an issue body whose declaration line sits inside a
// backtick fenced code block, preceded by "Dependencies: none" outside it.
func fencedBody(declaration string) string {
	fence := strings.Repeat("`", 3)
	return strings.Join([]string{"Dependencies: none", "", fence, declaration, fence}, "\n")
}

func TestCheckCircularDependenciesFencedCode(t *testing.T) {
	const selfRef = "Dependencies: #10"
	details := map[int]*github.IssueDetails{
		10: {State: "open", Body: selfRef},
	}

	t.Run("fenced self reference yields no edge", func(t *testing.T) {
		var calls []int
		dv := NewDependencyValidator()
		dv.getIssueDetails = fakeIssueLookup(&calls, details, nil)

		got := dv.checkCircularDependencies("owner/repo", 10, fencedBody(selfRef), map[int]bool{})

		if len(got) != 0 {
			t.Errorf("checkCircularDependencies() = %v, expected empty (fenced line is not an edge)", got)
		}
		if len(calls) != 0 {
			t.Errorf("lookups = %v, expected none", calls)
		}
	})

	t.Run("unfenced self reference reports a cycle", func(t *testing.T) {
		var calls []int
		dv := NewDependencyValidator()
		dv.getIssueDetails = fakeIssueLookup(&calls, details, nil)

		got := dv.checkCircularDependencies("owner/repo", 10, selfRef, map[int]bool{})

		if len(got) == 0 {
			t.Error("checkCircularDependencies() = empty, expected a cycle for an unfenced self reference")
		}
		if !reflect.DeepEqual(calls, []int{10}) {
			t.Errorf("lookups = %v, expected [10]", calls)
		}
	})
}

func TestValidateIssueFencedOnlyDependency(t *testing.T) {
	var calls []int
	dv := NewDependencyValidator()
	dv.getIssueDetails = fakeIssueLookup(&calls, nil, nil)

	result, err := dv.ValidateIssue("owner/repo", 10, fencedBody("Dependencies: #10"))
	if err != nil {
		t.Fatalf("ValidateIssue() returned unexpected error: %v", err)
	}

	if len(calls) != 0 {
		t.Errorf("lookups = %v, expected none", calls)
	}
	if !result.IsValid {
		t.Errorf("IsValid = false, expected true (only dependency is inside a fence)")
	}
	if len(result.UnresolvedDependencies) != 0 {
		t.Errorf("UnresolvedDependencies = %v, expected empty", result.UnresolvedDependencies)
	}
	if len(result.CircularDependencies) != 0 {
		t.Errorf("CircularDependencies = %v, expected empty", result.CircularDependencies)
	}
}
