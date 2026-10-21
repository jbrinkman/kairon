package eval

import (
	"reflect"
	"strings"
	"testing"
)

func mustFilter(t *testing.T, expr string) Filter {
	t.Helper()
	f, err := ParseFilter(expr)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", expr, err)
	}
	return f
}

func TestFilterParseOperators(t *testing.T) {
	tests := []struct {
		expr string
		want Filter
	}{
		{"speed==fast", Filter{Key: "speed", Op: "==", Values: []string{"fast"}}},
		{"speed<>fast", Filter{Key: "speed", Op: "<>", Values: []string{"fast"}}},
		{"speed!=fast", Filter{Key: "speed", Op: "!=", Values: []string{"fast"}}},
		{"n<3", Filter{Key: "n", Op: "<", Values: []string{"3"}}},
		{"n<=3", Filter{Key: "n", Op: "<=", Values: []string{"3"}}},
		{"n>3", Filter{Key: "n", Op: ">", Values: []string{"3"}}},
		{"n>=3", Filter{Key: "n", Op: ">=", Values: []string{"3"}}},
		{"color<>red,blue", Filter{Key: "color", Op: "<>", Values: []string{"red", "blue"}}},
		{"color==red,blue,green", Filter{Key: "color", Op: "==", Values: []string{"red", "blue", "green"}}},
		{"color!=red,blue", Filter{Key: "color", Op: "!=", Values: []string{"red", "blue"}}},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			got, err := ParseFilter(tt.expr)
			if err != nil {
				t.Fatalf("ParseFilter(%q): %v", tt.expr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFilter(%q) = %+v, want %+v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestFilterParseTrimsWhitespace(t *testing.T) {
	got := mustFilter(t, "  speed  ==  fast ,  slow  ")
	want := Filter{Key: "speed", Op: "==", Values: []string{"fast", "slow"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	got = mustFilter(t, " n >= 3 ")
	want = Filter{Key: "n", Op: ">=", Values: []string{"3"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFilterParseOperatorChoosesLongest(t *testing.T) {
	// "<=" wins over "<", so the value is ">b".
	got := mustFilter(t, "a<=>b")
	want := Filter{Key: "a", Op: "<=", Values: []string{">b"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFilterParseErrors(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string // extra text the message must carry
	}{
		{"no operator", "speedfast", "operator"},
		{"empty string", "", "operator"},
		{"single equals", "speed=fast", "operator"},
		{"bang alone", "speed!fast", "operator"},
		{"arrow equals", "a=>b", "operator"},
		{"equals less", "a=<b", "operator"},
		{"empty key", "==fast", "key"},
		{"blank key", "   !=fast", "key"},
		{"empty value eq", "speed==", "value"},
		{"empty value ne", "speed<>", "value"},
		{"blank value", "speed==   ", "value"},
		{"empty value ordering", "n>=", "value"},
		{"leading empty item", "speed==,fast", "empty"},
		{"trailing empty item", "speed==fast,", "empty"},
		{"middle empty item", "speed==fast,,slow", "empty"},
		{"blank item", "speed<>fast, ,slow", "empty"},
		{"list with <", "n<1,2", "one value"},
		{"list with <=", "n<=1,2", "one value"},
		{"list with >", "n>1,2", "one value"},
		{"list with >=", "n>=1,2", "one value"},
	}
	const syntax = "<key><operator><value>"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFilter(tt.expr)
			if err == nil {
				t.Fatalf("ParseFilter(%q) succeeded, want an error", tt.expr)
			}
			msg := err.Error()
			if !strings.Contains(msg, syntax) {
				t.Errorf("error %q does not show the expected syntax %q", msg, syntax)
			}
			for _, op := range []string{"==", "<>", "!=", "<=", ">="} {
				if !strings.Contains(msg, op) {
					t.Errorf("error %q does not list operator %s", msg, op)
				}
			}
			if !strings.Contains(msg, `"`+tt.expr+`"`) {
				t.Errorf("error %q does not quote the bad input %q", msg, tt.expr)
			}
			if !strings.Contains(msg, tt.want) {
				t.Errorf("error %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestFilterParseFilters(t *testing.T) {
	if fs, err := ParseFilters(nil); fs != nil || err != nil {
		t.Errorf("ParseFilters(nil) = %v, %v; want nil, nil", fs, err)
	}
	if fs, err := ParseFilters([]string{}); fs != nil || err != nil {
		t.Errorf("ParseFilters(empty) = %v, %v; want nil, nil", fs, err)
	}
	fs, err := ParseFilters([]string{"a==1", "b<>2,3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 || fs[0].Key != "a" || fs[1].Key != "b" {
		t.Errorf("filters = %+v", fs)
	}
	// The first error wins, and it names the offending expression.
	_, err = ParseFilters([]string{"a==1", "bad", "worse="})
	if err == nil || !strings.Contains(err.Error(), `"bad"`) {
		t.Errorf("err = %v, want the first bad expression \"bad\"", err)
	}
}

func TestFilterStringAndFormat(t *testing.T) {
	f := mustFilter(t, " color <> red , blue ")
	if got := f.String(); got != "color<>red,blue" {
		t.Errorf("String() = %q", got)
	}
	g := mustFilter(t, "n>=2")
	if got := FormatFilters([]Filter{f, g}); got != "color<>red,blue AND n>=2" {
		t.Errorf("FormatFilters = %q", got)
	}
	if got := FormatFilters([]Filter{g}); got != "n>=2" {
		t.Errorf("FormatFilters(one) = %q", got)
	}
	if got := FormatFilters(nil); got != "" {
		t.Errorf("FormatFilters(nil) = %q, want empty", got)
	}
}

func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestFilterMatchEquality(t *testing.T) {
	look := mapLookup(map[string]string{"speed": "fast", "color": "red"})
	tests := []struct {
		expr string
		want bool
	}{
		{"speed==fast", true},
		{"speed==slow", false},
		{"speed==Fast", false}, // case-sensitive
		{"speed<>Fast", true},
		{"speed!=Fast", true},
		{"speed==slow,fast", true}, // any listed value
		{"speed==slow,medium", false},
		{"speed<>slow,medium", true}, // none of the listed values
		{"speed!=slow,medium", true},
		{"speed<>slow,fast", false},
		{"speed!=fast,slow", false},
		{"speed<>fast", false},
		{"speed!=fast", false},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			if got := mustFilter(t, tt.expr).matches(look); got != tt.want {
				t.Errorf("%s matches = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestFilterMatchMissingKeyNeverMatches(t *testing.T) {
	look := mapLookup(map[string]string{"speed": "fast"})
	for _, expr := range []string{
		"absent==x", "absent<>x", "absent!=x", "absent<1", "absent<=1", "absent>1", "absent>=1",
		"absent==x,y", "absent<>x,y",
	} {
		t.Run(expr, func(t *testing.T) {
			if mustFilter(t, expr).matches(look) {
				t.Errorf("%s matched a case without that key", expr)
			}
		})
	}
}

func TestFilterMatchOrderingNumeric(t *testing.T) {
	tests := []struct {
		have string
		expr string
		want bool
	}{
		{"2", "n<10", true}, // numeric, not textual ("2" > "10" as text)
		{"10", "n<2", false},
		{"10", "n>2", true},
		{"2", "n>10", false},
		{"3", "n<=3", true},
		{"3", "n>=3", true},
		{"3", "n<3", false},
		{"3", "n>3", false},
		{"3.0", "n>=3", true},
		{"3", "n<=3.0", true},
		{"-1.5", "n<0", true},
		{"+2", "n>1", true},
		{".5", "n<1", true},
		{"5.", "n>4", true},
		{"1e3", "n>999", true},
		{"1E3", "n>=1000", true},
		{"2.5e-1", "n<1", true},
		// A non-numeric case value never matches an ordering operator.
		{"fast", "n<10", false},
		{"fast", "n>=0", false},
		{"", "n<1", false},
		{" 3", "n>1", false},
		{"1,5", "n>1", false},
		{"1_000", "n>1", false},
		// A non-numeric filter value never matches.
		{"3", "n<abc", false},
		{"3", "n>=x", false},
		// NaN, Inf and hex are not numeric, on either side.
		{"NaN", "n<1", false},
		{"nan", "n>=0", false},
		{"Inf", "n>1", false},
		{"+Inf", "n>1", false},
		{"-Inf", "n<1", false},
		{"infinity", "n>1", false},
		{"0x10", "n>1", false},
		{"0x10", "n>=16", false},
		{"3", "n<NaN", false},
		{"3", "n<Inf", false},
		{"3", "n<0x10", false},
		{"1e999", "n>1", false}, // overflows to Inf
	}
	for _, tt := range tests {
		t.Run(tt.have+"|"+tt.expr, func(t *testing.T) {
			look := mapLookup(map[string]string{"n": tt.have})
			if got := mustFilter(t, tt.expr).matches(look); got != tt.want {
				t.Errorf("n=%q %s matches = %v, want %v", tt.have, tt.expr, got, tt.want)
			}
		})
	}
}

func TestFilterEqualityNeverParsesNumbers(t *testing.T) {
	look := mapLookup(map[string]string{"retries": "3"})
	if mustFilter(t, "retries==3.0").matches(look) {
		t.Error("retries==3.0 matched \"3\": equality must be textual")
	}
	if mustFilter(t, "retries==03").matches(look) {
		t.Error("retries==03 matched \"3\": equality must be textual")
	}
	if !mustFilter(t, "retries<>3.0").matches(look) {
		t.Error("retries<>3.0 did not match \"3\": the texts differ")
	}
	if !mustFilter(t, "retries==3").matches(look) {
		t.Error("retries==3 did not match \"3\"")
	}
}

func TestFilterMatchAllIsAnd(t *testing.T) {
	look := mapLookup(map[string]string{"speed": "fast", "n": "5"})
	pass := []Filter{mustFilter(t, "speed==fast"), mustFilter(t, "n>=5")}
	if !matchAll(pass, look) {
		t.Error("matchAll with two matching filters = false")
	}
	mixed := []Filter{mustFilter(t, "speed==fast"), mustFilter(t, "n>5")}
	if matchAll(mixed, look) {
		t.Error("matchAll with one failing filter = true")
	}
	if !matchAll(nil, look) {
		t.Error("matchAll with no filters = false, want true")
	}
}

func filterFixtureCases() []TestCase {
	return []TestCase{
		{Name: "a", Agent: "alpha", Metadata: CaseMetadata{"speed": "fast", "skill": "cli", "n": "1"}},
		{Name: "b", Agent: "alpha", Metadata: CaseMetadata{"speed": "slow", "skill": "web", "n": "10"}},
		{Name: "c", Agent: "alpha"},
		{Name: "d", Agent: "alpha", Metadata: CaseMetadata{"speed": "fast", "n": "2"}},
	}
}

func names(cases []TestCase) []string {
	var out []string
	for _, c := range cases {
		out = append(out, c.Name)
	}
	return out
}

func TestFilterBuiltInKeysAndMetadataKeys(t *testing.T) {
	tc := TestCase{Name: "n1", Agent: "alpha", Metadata: CaseMetadata{"skill": "cli", "speed": "fast"}}
	for key, want := range map[string]string{"agent": "alpha", "name": "n1", "skill": "cli", "speed": "fast"} {
		got, ok := tc.filterValue(key)
		if !ok || got != want {
			t.Errorf("filterValue(%q) = %q, %v; want %q, true", key, got, ok, want)
		}
	}
	if v, ok := tc.filterValue("absent"); ok {
		t.Errorf("filterValue(absent) = %q, true; want missing", v)
	}

	// agent and name come from the case and are always present, even for a
	// case with no metadata; skill is not special and is missing without it.
	bare := TestCase{Name: "n2", Agent: "beta"}
	if v, ok := bare.filterValue("agent"); !ok || v != "beta" {
		t.Errorf("bare agent = %q, %v", v, ok)
	}
	if v, ok := bare.filterValue("name"); !ok || v != "n2" {
		t.Errorf("bare name = %q, %v", v, ok)
	}
	if v, ok := bare.filterValue("skill"); ok {
		t.Errorf("bare skill = %q, true; want missing", v)
	}
}

func TestFilterCases(t *testing.T) {
	cases := filterFixtureCases()
	tests := []struct {
		name  string
		exprs []string
		want  []string
	}{
		{"no filters keeps all", nil, []string{"a", "b", "c", "d"}},
		{"equality", []string{"speed==fast"}, []string{"a", "d"}},
		{"not equal skips missing key", []string{"speed<>fast"}, []string{"b"}},
		{"bang equal skips missing key", []string{"speed!=fast"}, []string{"b"}},
		{"and of two", []string{"speed==fast", "skill==cli"}, []string{"a"}},
		{"and with no result", []string{"speed==slow", "skill==cli"}, nil},
		{"name built-in with list", []string{"name==a,c"}, []string{"a", "c"}},
		{"agent built-in", []string{"agent==alpha"}, []string{"a", "b", "c", "d"}},
		{"agent built-in other", []string{"agent==beta"}, nil},
		{"skill from metadata", []string{"skill==web"}, []string{"b"}},
		{"numeric ordering", []string{"n>=2"}, []string{"b", "d"}},
		{"numeric ordering with equality", []string{"n<10", "speed==fast"}, []string{"a", "d"}},
		{"missing key matches nothing", []string{"nope==x"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := ParseFilters(tt.exprs)
			if err != nil {
				t.Fatal(err)
			}
			got := names(filterCases(cases, fs))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterCases = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterCasesDoesNotMutateInput(t *testing.T) {
	cases := filterFixtureCases()
	fs, _ := ParseFilters([]string{"speed==fast"})
	_ = filterCases(cases, fs)
	if got := names(cases); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Errorf("input changed: %v", got)
	}
}

func TestFilterResultLookup(t *testing.T) {
	cr := CaseResult{CaseName: "case-1", Metadata: CaseMetadata{"speed": "fast", "skill": "cli"}}
	look := resultLookup("alpha", cr)
	for key, want := range map[string]string{"agent": "alpha", "name": "case-1", "speed": "fast", "skill": "cli"} {
		got, ok := look(key)
		if !ok || got != want {
			t.Errorf("lookup(%q) = %q, %v; want %q, true", key, got, ok, want)
		}
	}
	if v, ok := look("absent"); ok {
		t.Errorf("lookup(absent) = %q, true", v)
	}

	// A result saved without metadata matches only agent and name.
	legacy := resultLookup("alpha", CaseResult{CaseName: "old"})
	if !mustFilter(t, "agent==alpha").matches(legacy) || !mustFilter(t, "name==old").matches(legacy) {
		t.Error("legacy result does not match built-in keys")
	}
	if mustFilter(t, "speed<>fast").matches(legacy) {
		t.Error("legacy result matched a metadata filter")
	}
}
