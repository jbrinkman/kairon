package watcher

import (
	"errors"
	"reflect"
	"sort"
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
