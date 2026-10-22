package eval

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/stageoutput"
	"gopkg.in/yaml.v3"
)

// These tests tie the shipped planner suite to the real output boundary in
// internal/planner. The suite's own checks are regexes; here every reference
// document it ships is run through the real extractor, the mode contract and
// the guard chain, and the automated cases are pinned to the preamble constant
// the boundary's callers use. Nothing under .kairon/evals is edited or copied.

// plannerCaseMode is the planner mode a shipped case exercises. Automated
// cases are the automated-*.yaml files; every other case is interactive.
func plannerCaseMode(name string) planner.Mode {
	if strings.HasPrefix(name, "automated-") {
		return planner.ModeAutomated
	}
	return planner.ModeInteractive
}

// plannerRequestFor builds the boundary request for one stub reply. For an
// automated case the issue number is the document's own source ref, which the
// mode contract then requires to be a github_issue reference.
func plannerRequestFor(t *testing.T, name, reply string) planner.Request {
	t.Helper()
	req := planner.Request{Mode: plannerCaseMode(name), Replies: []string{reply}}
	if req.Mode != planner.ModeAutomated {
		return req
	}
	raw, err := planner.Extract(reply)
	if err != nil {
		t.Fatalf("%s: extract: %v", name, err)
	}
	doc, err := stageoutput.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("%s: parse: %v", name, err)
	}
	n, err := strconv.Atoi(doc.Source.Ref)
	if err != nil {
		t.Fatalf("%s: automated source ref %q is not an issue number: %v", name, doc.Source.Ref, err)
	}
	req.Issue = n
	return req
}

func TestPlannerBoundaryStubDocumentsPassTheBoundary(t *testing.T) {
	p := plannerSuitePaths(t)
	docs := stubDocuments(t, p)

	var checked int
	for _, name := range plannerCaseNames {
		for i, reply := range docs[name] {
			checked++
			if _, err := planner.Extract(reply); err != nil {
				t.Errorf("%s reply %d: planner.Extract: %v", name, i, err)
				continue
			}
			// Check runs extraction, the mode contract and guard.DefaultChain,
			// and writes nothing: the Boundary has no store.
			res, err := (&planner.Boundary{}).Check(plannerRequestFor(t, name, reply))
			if err != nil {
				t.Errorf("%s reply %d: Check: %v", name, i, err)
				continue
			}
			if res.Outcome != planner.OutcomeChecked {
				t.Errorf("%s reply %d: outcome = %v, want checked; failure: %+v", name, i, res.Outcome, res.Failure)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no stub document found in the planner suite; the test checked nothing")
	}
}

// TestPlannerBoundaryAutomatedInputsStartWithIntakeLine reads the shipped
// YAML, not a copy of the line, so editing a case's first line makes it fail.
func TestPlannerBoundaryAutomatedInputsStartWithIntakeLine(t *testing.T) {
	p := plannerSuitePaths(t)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: p.evals}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	cases, err := loadCases("planner")
	if err != nil {
		t.Fatal(err)
	}

	var automated int
	for _, tc := range cases {
		auto := strings.HasPrefix(tc.Name, "automated-")
		first, _, _ := strings.Cut(tc.Input, "\n")
		if auto {
			automated++
			if first != planner.AutomatedIntakeLine {
				t.Errorf("%s: first input line = %q, want planner.AutomatedIntakeLine %q", tc.Name, first, planner.AutomatedIntakeLine)
			}
			continue
		}
		// An interactive case must not select automated mode.
		if strings.Contains(tc.Input, planner.AutomatedIntakeLine) {
			t.Errorf("%s: interactive case input contains the automated intake line", tc.Name)
		}
	}
	if automated == 0 {
		t.Fatal("no automated-*.yaml case found; the test checked nothing")
	}
}

// TestPlannerBoundarySameDocumentType: both modes return the same document.
// The mode is chosen by the prompt, and nothing in the document records it.
func TestPlannerBoundarySameDocumentType(t *testing.T) {
	p := plannerSuitePaths(t)
	docs := stubDocuments(t, p)

	parse := func(name string) (*stageoutput.Document, map[string]any) {
		t.Helper()
		if len(docs[name]) != 1 {
			t.Fatalf("%s: %d stub documents, want 1", name, len(docs[name]))
		}
		raw, err := planner.Extract(docs[name][0])
		if err != nil {
			t.Fatalf("%s: extract: %v", name, err)
		}
		doc, err := stageoutput.Parse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		var keys map[string]any
		if err := yaml.Unmarshal([]byte(raw), &keys); err != nil {
			t.Fatalf("%s: yaml: %v", name, err)
		}
		return doc, keys
	}

	inter, interKeys := parse("interactive-return-document")
	auto, autoKeys := parse("automated-best-effort-fill")

	if a, b := reflect.TypeOf(inter), reflect.TypeOf(auto); a != b {
		t.Errorf("document types differ: interactive %v, automated %v", a, b)
	}
	if inter.SchemaVersion != auto.SchemaVersion {
		t.Errorf("schema_version differs: interactive %d, automated %d", inter.SchemaVersion, auto.SchemaVersion)
	}
	if inter.Stage != auto.Stage || inter.Stage != stageoutput.StagePlanner {
		t.Errorf("stage: interactive %q, automated %q, want both %q", inter.Stage, auto.Stage, stageoutput.StagePlanner)
	}
	if inter.Result != auto.Result || inter.Result != stageoutput.ResultCanonical {
		t.Errorf("result: interactive %q, automated %q, want both %q", inter.Result, auto.Result, stageoutput.ResultCanonical)
	}

	// No mode-specific key, in the documents or in the Go type.
	isModeKey := func(k string) bool {
		k = strings.ToLower(k)
		return strings.Contains(k, "mode") || strings.Contains(k, "interactive") || strings.Contains(k, "automated")
	}
	for label, keys := range map[string]map[string]any{"interactive": interKeys, "automated": autoKeys} {
		var names []string
		for k := range keys {
			names = append(names, k)
			if isModeKey(k) {
				t.Errorf("%s document has a mode-specific key %q", label, k)
			}
		}
		sort.Strings(names)
		t.Logf("%s document keys: %v", label, names)
	}
	typ := reflect.TypeOf(stageoutput.Document{})
	for i := 0; i < typ.NumField(); i++ {
		if isModeKey(typ.Field(i).Name) {
			t.Errorf("stageoutput.Document has a mode-specific field %q", typ.Field(i).Name)
		}
	}
}
