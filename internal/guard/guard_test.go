package guard

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// fakeGuard is a recording Guard. It passes or rejects on demand, appends
// "admit:<name>" / "fail:<name>" to the shared log, and counts calls.
type fakeGuard struct {
	name       string
	reject     bool
	failureErr error
	// result, when non-nil, overrides what Admit returns (for contract tests).
	result *AdmissionResult
	log    *[]string

	admitCalls int
	failCalls  int
	failCtxs   []FailureContext
	admitted   []Input
}

func (g *fakeGuard) Admit(in Input) AdmissionResult {
	g.admitCalls++
	g.admitted = append(g.admitted, in)
	if g.log != nil {
		*g.log = append(*g.log, "admit:"+g.name)
	}
	if g.result != nil {
		return *g.result
	}
	if g.reject {
		return in.Reject(g.name, "expected-"+g.name, "actual-"+g.name)
	}
	return Admitted()
}

func (g *fakeGuard) OnFailure(fc FailureContext) error {
	g.failCalls++
	g.failCtxs = append(g.failCtxs, fc)
	if g.log != nil {
		*g.log = append(*g.log, "fail:"+g.name)
	}
	return g.failureErr
}

func validInput() Input {
	return Input{Stage: stageoutput.StagePlanner, Raw: "raw", Attempt: Attempt{Current: 2, Max: 3}}
}

func TestGuard_InterfaceShape(t *testing.T) {
	typ := reflect.TypeOf((*Guard)(nil)).Elem()
	require.Equal(t, 2, typ.NumMethod(), "Guard must have exactly two methods")

	admit, ok := typ.MethodByName("Admit")
	require.True(t, ok, "Guard.Admit missing")
	assert.Equal(t, reflect.TypeOf(func(Input) AdmissionResult { return AdmissionResult{} }), admit.Type)

	onFailure, ok := typ.MethodByName("OnFailure")
	require.True(t, ok, "Guard.OnFailure missing")
	assert.Equal(t, reflect.TypeOf(func(FailureContext) error { return nil }), onFailure.Type)
}

func TestInput_HasNoMutableFields(t *testing.T) {
	var walk func(path string, typ reflect.Type)
	walk = func(path string, typ reflect.Type) {
		switch typ.Kind() {
		case reflect.String, reflect.Int:
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				walk(path+"."+f.Name, f.Type)
			}
		default:
			t.Errorf("%s has kind %s; Input may hold only strings, ints and structs of those", path, typ.Kind())
		}
	}
	walk("Input", reflect.TypeOf(Input{}))
}

func TestAdmission_TrivialGuard(t *testing.T) {
	in := validInput()

	pass := (&fakeGuard{name: "g"}).Admit(in)
	assert.True(t, pass.Pass)
	assert.Nil(t, pass.Failure)
	assert.NoError(t, pass.Validate())

	fail := (&fakeGuard{name: "g", reject: true}).Admit(in)
	assert.False(t, fail.Pass)
	require.NotNil(t, fail.Failure)
	assert.NoError(t, fail.Validate())
}

func TestAdmissionResult_Validate(t *testing.T) {
	fc := validInput().Reject("g", "e", "a").Failure
	require.NotNil(t, fc)

	tests := []struct {
		name    string
		res     AdmissionResult
		wantErr bool
	}{
		{"pass with nil failure", AdmissionResult{Pass: true}, false},
		{"fail with failure", AdmissionResult{Failure: fc}, false},
		{"pass with failure", AdmissionResult{Pass: true, Failure: fc}, true},
		{"fail without failure", AdmissionResult{Pass: false}, true},
		{"fail with invalid failure", AdmissionResult{Failure: &FailureContext{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.res.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalid)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestFailureContext_AllFieldsPopulated(t *testing.T) {
	res := validInput().Reject("g", "exp", "act")
	require.False(t, res.Pass)
	require.NotNil(t, res.Failure)
	fc := *res.Failure

	assert.Equal(t, stageoutput.StagePlanner, fc.Stage)
	assert.Equal(t, "g", fc.Guard)
	assert.Equal(t, 2, fc.Attempt.Current)
	assert.Equal(t, 3, fc.Attempt.Max)
	assert.Equal(t, "exp", fc.Expected)
	assert.Equal(t, "act", fc.Actual)
	assert.NoError(t, fc.Validate())

	tests := []struct {
		name   string
		mutate func(*FailureContext)
	}{
		{"empty stage", func(f *FailureContext) { f.Stage = "" }},
		{"unknown stage", func(f *FailureContext) { f.Stage = "nope" }},
		{"empty guard", func(f *FailureContext) { f.Guard = "" }},
		{"current zero", func(f *FailureContext) { f.Attempt.Current = 0 }},
		{"current above max", func(f *FailureContext) { f.Attempt.Current = 4 }},
		{"max zero", func(f *FailureContext) { f.Attempt = Attempt{} }},
		{"empty expected", func(f *FailureContext) { f.Expected = "" }},
		{"empty actual", func(f *FailureContext) { f.Actual = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := fc
			tt.mutate(&bad)
			err := bad.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestAttempt_Valid(t *testing.T) {
	assert.True(t, Attempt{1, 1}.Valid())
	assert.True(t, Attempt{2, 3}.Valid())
	assert.False(t, Attempt{0, 3}.Valid())
	assert.False(t, Attempt{4, 3}.Valid())
	assert.False(t, Attempt{-1, 3}.Valid())
	assert.False(t, Attempt{}.Valid())
}

func TestInput_Validate(t *testing.T) {
	assert.NoError(t, validInput().Validate())

	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{"empty stage", func(i *Input) { i.Stage = "" }},
		{"unknown stage", func(i *Input) { i.Stage = "nope" }},
		{"attempt zero", func(i *Input) { i.Attempt = Attempt{0, 3} }},
		{"attempt over max", func(i *Input) { i.Attempt = Attempt{4, 3} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			err := in.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestInput_Document(t *testing.T) {
	raw, err := stageoutput.Marshal(&stageoutput.Document{
		SchemaVersion: stageoutput.CurrentSchemaVersion,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: "142"},
		Generated:     "2026-10-20T20:00:00Z",
		Body:          "## Problem\nx\n",
	})
	require.NoError(t, err)

	in := validInput()
	in.Raw = string(raw)
	doc, err := in.Document()
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "## Problem\nx\n", doc.Body)

	// Each call returns its own copy.
	doc2, err := in.Document()
	require.NoError(t, err)
	assert.NotSame(t, doc, doc2)

	in.Raw = "not: [a document"
	doc, err = in.Document()
	assert.Error(t, err)
	assert.Nil(t, doc)
}

func TestErrors_EmptyChainMatchesInvalid(t *testing.T) {
	assert.ErrorIs(t, ErrEmptyChain, ErrInvalid)
	assert.NotErrorIs(t, ErrNoFailureHandler, ErrInvalid)
}
