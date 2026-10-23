package planner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// DependenciesFormName is the name DependenciesForm reports in
// guard.FailureContext.Guard.
const DependenciesFormName = "dependencies_form"

const dependenciesExpected = "the body has exactly one single-line `Dependencies: #12, #15` (or `Dependencies: none`), " +
	"every reference is #<number> with no unresolved #{key} placeholder, and no bullet list under the Dependencies heading"

var (
	// depsHeadingRE matches a markdown heading that names the Dependencies
	// section.
	depsHeadingRE = regexp.MustCompile(`(?i)^ {0,3}(#{1,6})[ \t]+dependencies\b`)
	// anyHeadingRE matches any markdown heading and captures its level.
	anyHeadingRE = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+\S`)
	// depsFenceRE matches the opening or closing line of a fenced code block.
	depsFenceRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	// depsLineRE matches a line that is a Dependencies line: the keyword
	// starts the line. The value is everything after the colon.
	depsLineRE = regexp.MustCompile(`(?i)^[ \t]*dependencies:(.*)$`)
	// depsKeywordRE matches the keyword anywhere, to spot a bullet that
	// carries it.
	depsKeywordRE = regexp.MustCompile(`(?i)\bdependencies:`)
	// depsBulletRE matches a markdown list item.
	depsBulletRE = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)])[ \t]+`)
	// depsPlaceholderRE matches a "#{key}" placeholder token.
	depsPlaceholderRE = regexp.MustCompile(`#\{[^}\n]*\}`)
	// depsNumberRE matches one reference: #<digits>.
	depsNumberRE = regexp.MustCompile(`^#[0-9]+$`)
)

// DependenciesForm is a planner-side guard that checks the Dependencies
// section of a canonical body is in the one form the watcher can parse. The
// merged structural guard only checks that a Dependencies section exists, so
// this guard closes the rest: a body whose dependencies the watcher would
// read as none (a bullet list), as an unresolvable reference (a #{key} left
// behind), or not at all (no Dependencies: line) never reaches the commit
// step.
//
// A punch_out document passes: it has no body to check. The guard is meant to
// run after dependency substitution, so a "#{key}" still present in Admit is
// an unresolved reference.
type DependenciesForm struct {
	handler guard.FailureHandler
}

// NewDependenciesForm returns a DependenciesForm that calls handler from
// OnFailure. A nil handler makes OnFailure return guard.ErrNoFailureHandler.
func NewDependenciesForm(handler guard.FailureHandler) *DependenciesForm {
	return &DependenciesForm{handler: handler}
}

// Admit implements guard.Guard. It is pure: it reads in and returns a result.
func (g *DependenciesForm) Admit(in guard.Input) guard.AdmissionResult {
	doc, err := in.Document()
	if err != nil {
		return in.Reject(DependenciesFormName,
			"document parses under the strict stage-output schema (stageoutput.Parse)", err.Error())
	}
	if doc.Result != stageoutput.ResultCanonical {
		return guard.Admitted()
	}
	if actual := checkDependencies(doc.Body); actual != "" {
		return in.Reject(DependenciesFormName, dependenciesExpected, actual)
	}
	return guard.Admitted()
}

// OnFailure implements guard.Guard. It passes fc to the injected handler and
// returns the handler's error, or guard.ErrNoFailureHandler when none was
// supplied so a failure is never swallowed silently.
func (g *DependenciesForm) OnFailure(fc guard.FailureContext) error {
	if g.handler == nil {
		return guard.ErrNoFailureHandler
	}
	return g.handler(fc)
}

// checkDependencies returns a description of the first problem with the
// Dependencies form of body, or "" when the form is correct. Fenced code is
// ignored, so a body may show an example of a wrong form.
func checkDependencies(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	inFence := make([]bool, len(lines))
	var mark string
	for i, line := range lines {
		m := depsFenceRE.FindStringSubmatch(line)
		switch {
		case m != nil && mark == "":
			mark, inFence[i] = m[1], true
		case m != nil && m[1][0] == mark[0] && len(m[1]) >= len(mark) && strings.TrimSpace(line[len(m[0]):]) == "":
			mark, inFence[i] = "", true
		default:
			inFence[i] = mark != ""
		}
	}

	// A list under the Dependencies heading, or a list item that carries the
	// keyword, is the bullet form the watcher does not parse.
	for i, line := range lines {
		if inFence[i] {
			continue
		}
		if m := depsHeadingRE.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			for j := i + 1; j < len(lines); j++ {
				if inFence[j] {
					continue
				}
				if h := anyHeadingRE.FindStringSubmatch(lines[j]); h != nil && len(h[1]) <= level {
					break
				}
				if depsBulletRE.MatchString(lines[j]) {
					return fmt.Sprintf("a bullet list under the Dependencies heading (line %d: %q); "+
						"the watcher parses zero dependencies from it", j+1, strings.TrimSpace(lines[j]))
				}
			}
		}
		if depsBulletRE.MatchString(line) && depsKeywordRE.MatchString(line) {
			return fmt.Sprintf("a bullet that carries Dependencies: (line %d: %q); the line must not be a list item",
				i+1, strings.TrimSpace(line))
		}
	}

	var values []string
	for i, line := range lines {
		if inFence[i] {
			continue
		}
		if m := depsLineRE.FindStringSubmatch(line); m != nil {
			values = append(values, strings.TrimSpace(m[1]))
		}
	}
	switch len(values) {
	case 0:
		return "no Dependencies: line (write `Dependencies: none` when there are none)"
	case 1:
	default:
		return fmt.Sprintf("%d Dependencies: lines, want exactly one", len(values))
	}

	value := values[0]
	if p := depsPlaceholderRE.FindString(value); p != "" {
		return fmt.Sprintf("unresolved placeholder %s on the Dependencies: line (%q)", p, value)
	}
	if strings.EqualFold(value, "none") {
		return ""
	}
	if value == "" {
		return "malformed Dependencies: line: it has no value (write `none` or `#12, #15`)"
	}
	for _, part := range strings.Split(value, ",") {
		if !depsNumberRE.MatchString(strings.TrimSpace(part)) {
			return fmt.Sprintf("malformed Dependencies: line: %q is neither none nor a comma-separated list of #<number> (%q)",
				strings.TrimSpace(part), value)
		}
	}
	return ""
}
