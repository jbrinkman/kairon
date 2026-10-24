package watcher

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jbrinkman/kairon/internal/github"
)

// Compiled regexes for dependency parsing
var (
	dependsOnIssueRe = regexp.MustCompile(`(?i)depends\s+on\s+issue\s+#(\d+)`)
	blockedByRe      = regexp.MustCompile(`(?i)blocked\s+by:\s*#(\d+)`)
	dependsOnLinkRe  = regexp.MustCompile(`(?i)depends\s+on\s+\[issue\s+#(\d+)\]`)
	dependenciesRe   = regexp.MustCompile(`(?i)dependencies:\s*(.+?)(?:\n|$)`)
)

// DependencyParser extracts issue numbers from issue body text
type DependencyParser struct{}

// DependencyBackoff tracks backoff state for a single issue
type DependencyBackoff struct {
	issueNumber    int
	failureCount   int
	nextCheckRound int
}

// BackoffTracker manages backoff tracking for multiple issues
type BackoffTracker struct {
	backoffs     map[int]*DependencyBackoff
	currentRound int
	mu           sync.RWMutex
}

// ParseDependencies extracts all issue numbers from issue body text using multiple formats
func (dp *DependencyParser) ParseDependencies(issueBody string) []int {
	return dp.extractIssueNumbers(issueBody)
}

// fenceRE matches the opening or closing line of a fenced code block. It is
// the same rule used by depsFenceRE in internal/planner/deps_guard.go and by
// scanLines in internal/guard/structural.go; keep the three aligned.
var fenceRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

// fenceSentinel replaces each line inside a fenced code block. It must be
// non-whitespace so the dependency regexes (notably dependenciesRe, whose
// "dependencies:\s*(.+?)" can otherwise span consecutive blank lines) cannot
// match across a stripped fence and swallow a real declaration that follows
// it. NUL never appears in GitHub issue bodies and contains no digit or
// dependency keyword, so it is inert to every parse path.
const fenceSentinel = "\x00"

// stripFencedCode returns text with every line inside a fenced code block
// (delimiters included) replaced by a single-character sentinel line, so
// dependency examples in code fences are not parsed as real declarations and
// cannot bridge a label above the fence to a real declaration below it.
//
// A closing line must use the same character as the opener, be at least as
// long, and carry only whitespace after the run; an unclosed fence runs to the
// end of text. Text is split on "\n" only (a trailing "\r" is tolerated by the
// regex and by TrimSpace), so unfenced text is byte-identical and the line
// count is preserved. If no line looks like a fence, text is returned as is.
func stripFencedCode(text string) string {
	lines := strings.Split(text, "\n")

	hasFence := false
	for _, line := range lines {
		if fenceRE.MatchString(line) {
			hasFence = true
			break
		}
	}
	if !hasFence {
		return text
	}

	var mark string // opening run of the current fence; "" when outside one
	for i, line := range lines {
		m := fenceRE.FindStringSubmatch(line)
		switch {
		case m != nil && mark == "":
			mark = m[1]
			lines[i] = fenceSentinel
		case m != nil && m[1][0] == mark[0] && len(m[1]) >= len(mark) && strings.TrimSpace(line[len(m[0]):]) == "":
			mark = ""
			lines[i] = fenceSentinel
		case mark != "":
			lines[i] = fenceSentinel
		}
	}
	return strings.Join(lines, "\n")
}

// extractIssueNumbers finds issue numbers in text using various dependency formats
func (dp *DependencyParser) extractIssueNumbers(text string) []int {
	text = stripFencedCode(text)

	var issues []int
	seen := make(map[int]bool)

	// Match single-issue patterns
	for _, re := range []*regexp.Regexp{dependsOnIssueRe, blockedByRe, dependsOnLinkRe} {
		matches := re.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" {
				if num, err := strconv.Atoi(match[1]); err == nil && !seen[num] {
					issues = append(issues, num)
					seen[num] = true
				}
			}
		}
	}

	// Handle comma-separated "Dependencies: #N, #M" format
	depMatches := dependenciesRe.FindAllStringSubmatch(text, -1)
	for _, match := range depMatches {
		if len(match) > 1 {
			deps := strings.Split(match[1], ",")
			for _, dep := range deps {
				dep = strings.TrimSpace(dep)
				dep = strings.TrimPrefix(dep, "#")
				if num, err := strconv.Atoi(dep); err == nil && !seen[num] {
					issues = append(issues, num)
					seen[num] = true
				}
			}
		}
	}

	return issues
}

// NewBackoffTracker creates a new backoff tracker
func NewBackoffTracker() *BackoffTracker {
	return &BackoffTracker{
		backoffs:     make(map[int]*DependencyBackoff),
		currentRound: 0,
	}
}

// ShouldCheck returns true if the issue should be checked in the current round
func (bt *BackoffTracker) ShouldCheck(issueNumber int) bool {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	backoff, exists := bt.backoffs[issueNumber]
	if !exists {
		return true
	}

	return bt.currentRound >= backoff.nextCheckRound
}

// RecordFailure records a dependency check failure and calculates next check round
func (bt *BackoffTracker) RecordFailure(issueNumber int) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	backoff, exists := bt.backoffs[issueNumber]
	if !exists {
		backoff = &DependencyBackoff{
			issueNumber:  issueNumber,
			failureCount: 0,
		}
		bt.backoffs[issueNumber] = backoff
	}

	backoff.failureCount++

	// Calculate backoff delay: 2^failure_count rounds (max 16x)
	delay := 1 << backoff.failureCount
	if delay > 16 {
		delay = 16
	}

	backoff.nextCheckRound = bt.currentRound + delay
}

// GetRoundsUntilCheck returns how many rounds until the next check for an issue
func (bt *BackoffTracker) GetRoundsUntilCheck(issueNumber int) int {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	backoff, exists := bt.backoffs[issueNumber]
	if !exists {
		return 0
	}

	remaining := backoff.nextCheckRound - bt.currentRound
	if remaining < 0 {
		return 0
	}

	return remaining
}

// IncrementRound increments the current polling round
func (bt *BackoffTracker) IncrementRound() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	bt.currentRound++
}

// ValidationResult contains the result of dependency validation
type ValidationResult struct {
	IsValid                bool
	UnresolvedDependencies []int
	CircularDependencies   []int
}

// issueLookupFunc fetches issue details; defaults to github.GetIssueDetails.
type issueLookupFunc func(repo string, number int) (*github.IssueDetails, error)

// DependencyValidator validates issue dependencies
type DependencyValidator struct {
	parser          *DependencyParser
	getIssueDetails issueLookupFunc // unexported; tests may replace it
}

// NewDependencyValidator creates a new dependency validator
func NewDependencyValidator() *DependencyValidator {
	return &DependencyValidator{
		parser:          &DependencyParser{},
		getIssueDetails: github.GetIssueDetails,
	}
}

// ValidateIssue checks if an issue's dependencies are satisfied.
// issueBody is the pre-fetched issue body text to avoid an extra API call.
func (dv *DependencyValidator) ValidateIssue(repo string, issueNumber int, issueBody string) (*ValidationResult, error) {
	// Parse dependencies from issue body
	dependencies := dv.parser.ParseDependencies(issueBody)
	if len(dependencies) > 0 {
		log.Printf("[watcher] parsed %d dependencies for issue #%d: %v", len(dependencies), issueNumber, dependencies)
	}

	if len(dependencies) == 0 {
		return &ValidationResult{IsValid: true}, nil
	}

	// Fail fast: the watcher skips an issue on the first unresolved dependency,
	// so stop at the first one. Order is the order ParseDependencies returns.
	for _, dep := range dependencies {
		depDetails, err := dv.getIssueDetails(repo, dep)
		if err != nil {
			log.Printf("[watcher] error checking dependency #%d for issue #%d: %v", dep, issueNumber, err)
			return &ValidationResult{UnresolvedDependencies: []int{dep}}, nil
		}

		if !strings.EqualFold(depDetails.State, "closed") {
			log.Printf("[watcher] dependency #%d for issue #%d is in state '%s' (not closed)", dep, issueNumber, depDetails.State)
			return &ValidationResult{UnresolvedDependencies: []int{dep}}, nil
		}
	}

	return &ValidationResult{IsValid: true}, nil
}

// checkCircularDependencies detects circular dependencies.
// Accepts issueBody for the current node to avoid re-fetching it.
func (dv *DependencyValidator) checkCircularDependencies(repo string, issueNumber int, issueBody string, visited map[int]bool) []int {
	if visited[issueNumber] {
		return []int{issueNumber}
	}

	visited[issueNumber] = true

	dependencies := dv.parser.ParseDependencies(issueBody)
	for _, dep := range dependencies {
		// Fetch dependency body for recursive check
		depDetails, err := dv.getIssueDetails(repo, dep)
		if err != nil {
			continue
		}
		if circular := dv.checkCircularDependencies(repo, dep, depDetails.Body, visited); len(circular) > 0 {
			return append(circular, issueNumber)
		}
	}

	delete(visited, issueNumber)
	return nil
}
