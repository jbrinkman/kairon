package eval

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Filter is one parsed --filter expression: <key><operator><value>.
type Filter struct {
	Key    string
	Op     string   // as typed: "==", "<>", "!=", "<", "<=", ">", ">="
	Values []string // exactly one for an ordering operator
}

// filterOperators lists the operators longest first, so "<=" is not read as
// "<" followed by "=".
var filterOperators = []string{"==", "<>", "!=", "<=", ">=", "<", ">"}

const filterSyntax = "expected <key><operator><value> with operator one of ==, <>, !=, <, <=, >, >= " +
	"(for example speed==fast, color<>red,blue, retries>=2)"

func filterError(expr, problem string) error {
	return fmt.Errorf("invalid --filter %q: %s; %s", expr, problem, filterSyntax)
}

func isOrderingOp(op string) bool {
	return op == "<" || op == "<=" || op == ">" || op == ">="
}

// ParseFilter parses one filter expression. Whitespace around the key and
// around each value is trimmed. A comma-separated value list is allowed only
// for ==, <> and !=; ordering operators take exactly one value.
func ParseFilter(expr string) (Filter, error) {
	i := strings.IndexAny(expr, "=<>!")
	if i < 0 {
		return Filter{}, filterError(expr, "missing operator")
	}
	op := ""
	for _, candidate := range filterOperators {
		if strings.HasPrefix(expr[i:], candidate) {
			op = candidate
			break
		}
	}
	if op == "" {
		return Filter{}, filterError(expr, "unknown operator")
	}
	key := strings.TrimSpace(expr[:i])
	if key == "" {
		return Filter{}, filterError(expr, "empty key")
	}
	rest := expr[i+len(op):]

	if isOrderingOp(op) {
		if strings.Contains(rest, ",") {
			return Filter{}, filterError(expr, "operator "+op+" takes exactly one value, not a comma list")
		}
		v := strings.TrimSpace(rest)
		if v == "" {
			return Filter{}, filterError(expr, "empty value")
		}
		return Filter{Key: key, Op: op, Values: []string{v}}, nil
	}

	if strings.TrimSpace(rest) == "" {
		return Filter{}, filterError(expr, "empty value")
	}
	items := strings.Split(rest, ",")
	values := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			return Filter{}, filterError(expr, "empty item in the value list")
		}
		values = append(values, v)
	}
	return Filter{Key: key, Op: op, Values: values}, nil
}

// ParseFilters parses every expression; the first error wins. No expressions
// give nil, nil.
func ParseFilters(exprs []string) ([]Filter, error) {
	if len(exprs) == 0 {
		return nil, nil
	}
	fs := make([]Filter, 0, len(exprs))
	for _, e := range exprs {
		f, err := ParseFilter(e)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}
	// Sort into a canonical order so the recorded case_filter label (and the
	// resume integrity check that compares it) depends only on the set of
	// filters, not the order the flags were given. Matching is an AND over all
	// filters, so order never affects selection.
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].String() < fs[j].String() })
	return fs, nil
}

// String renders the filter in its normalized form, without whitespace.
func (f Filter) String() string {
	return f.Key + f.Op + strings.Join(f.Values, ",")
}

// FormatFilters renders filters joined with " AND ".
func FormatFilters(fs []Filter) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.String()
	}
	return strings.Join(parts, " AND ")
}

// decimalNumber is a plain decimal number: optional sign, digits with an
// optional fraction, optional exponent. strconv.ParseFloat alone also accepts
// NaN, Inf and hex floats, which are not numeric here.
var decimalNumber = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$`)

// parseDecimal returns the value of s when s is a plain, finite decimal number.
func parseDecimal(s string) (float64, bool) {
	if !decimalNumber.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// matches reports whether the case behind lookup satisfies the filter.
// lookup returns a key's text and whether the key is present. A missing key
// never matches, for any operator.
func (f Filter) matches(lookup func(key string) (string, bool)) bool {
	have, ok := lookup(f.Key)
	if !ok {
		return false
	}
	switch f.Op {
	case "==":
		for _, v := range f.Values {
			if have == v {
				return true
			}
		}
		return false
	case "<>", "!=":
		for _, v := range f.Values {
			if have == v {
				return false
			}
		}
		return true
	}
	// Ordering: both sides must be plain decimal numbers.
	if len(f.Values) != 1 {
		return false
	}
	a, aok := parseDecimal(have)
	b, bok := parseDecimal(f.Values[0])
	if !aok || !bok {
		return false
	}
	switch f.Op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

// matchAll reports whether every filter matches (AND). No filters match.
func matchAll(fs []Filter, lookup func(key string) (string, bool)) bool {
	for _, f := range fs {
		if !f.matches(lookup) {
			return false
		}
	}
	return true
}

// filterCases returns the cases that satisfy every filter, in order. With no
// filters it returns cases unchanged.
func filterCases(cases []TestCase, fs []Filter) []TestCase {
	if len(fs) == 0 {
		return cases
	}
	var out []TestCase
	for _, tc := range cases {
		if matchAll(fs, tc.filterValue) {
			out = append(out, tc)
		}
	}
	return out
}

// filterValue is the lookup for a case: agent and name are built-in keys;
// every other key (skill included) comes from the case's metadata.
func (tc TestCase) filterValue(key string) (string, bool) {
	switch key {
	case "agent":
		return tc.Agent, true
	case "name":
		return tc.Name, true
	}
	v, ok := tc.Metadata[key]
	return v, ok
}

// resultLookup is the lookup for a saved case result: agent is the agent the
// result belongs to, name is the case name, and every other key is the
// metadata recorded with the result. A result saved before metadata existed
// therefore matches only agent and name filters.
func resultLookup(agent string, cr CaseResult) func(string) (string, bool) {
	return func(key string) (string, bool) {
		switch key {
		case "agent":
			return agent, true
		case "name":
			return cr.CaseName, true
		}
		v, ok := cr.Metadata[key]
		return v, ok
	}
}
