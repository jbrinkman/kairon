package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// allCheckTypes is the vocabulary the documentation must cover. The test also
// derives the set from checkSpecs, so a type added to the code without being
// listed here (or documented) fails.
var allCheckTypes = []CheckType{
	CheckCommand,
	CheckFileExists,
	CheckFileAbsent,
	CheckFileContains,
	CheckFileNotContains,
	CheckChangedFiles,
	CheckOutputContains,
	CheckOutputNotContains,
	CheckGHLogContains,
	CheckGHLogNotContains,
}

var yamlFence = regexp.MustCompile("(?s)```yaml\n(.*?)\n```")

// docYAMLBlocks returns the contents of every ```yaml fenced block in doc.
func docYAMLBlocks(doc string) []string {
	var blocks []string
	for _, m := range yamlFence.FindAllStringSubmatch(doc, -1) {
		blocks = append(blocks, m[1])
	}
	return blocks
}

// documentedChecks returns, per type, the checks found in YAML examples of doc
// that contain a `checks:` list, together with the raw text of the block.
func documentedChecks(t *testing.T, doc string) map[CheckType][]documentedCheck {
	t.Helper()
	found := map[CheckType][]documentedCheck{}
	for _, block := range docYAMLBlocks(doc) {
		if !strings.Contains(block, "checks:") {
			continue
		}
		var parsed struct {
			Checks []Check `yaml:"checks"`
		}
		if err := yaml.Unmarshal([]byte(block), &parsed); err != nil {
			t.Fatalf("docs/evaluation.md: YAML example with a checks list does not parse: %v\n%s", err, block)
		}
		for _, c := range parsed.Checks {
			found[c.Type] = append(found[c.Type], documentedCheck{check: c, block: block})
		}
	}
	return found
}

type documentedCheck struct {
	check Check
	block string
}

func readEvaluationDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "evaluation.md"))
	if err != nil {
		t.Fatalf("reading docs/evaluation.md: %v", err)
	}
	return string(data)
}

func TestChecksDocsCoversEveryCheckType(t *testing.T) {
	doc := readEvaluationDoc(t)
	found := documentedChecks(t, doc)

	// Every type the code knows must be in the list this test asserts on.
	listed := map[CheckType]bool{}
	for _, ct := range allCheckTypes {
		listed[ct] = true
	}
	var inCode []string
	for ct := range checkSpecs {
		inCode = append(inCode, string(ct))
		if !listed[ct] {
			t.Errorf("check type %q is implemented but not covered by allCheckTypes in this test", ct)
		}
	}
	sort.Strings(inCode)
	if len(inCode) != len(allCheckTypes) {
		t.Errorf("checkSpecs has %d types %v, allCheckTypes has %d", len(inCode), inCode, len(allCheckTypes))
	}

	for _, ct := range allCheckTypes {
		ct := ct
		t.Run(string(ct), func(t *testing.T) {
			examples := found[ct]
			if len(examples) == 0 {
				t.Fatalf("docs/evaluation.md has no YAML example with `type: %s` in a checks list", ct)
			}
			line := regexp.MustCompile(`(?m)^\s*(-\s+)?type:\s+` + regexp.QuoteMeta(string(ct)) + `(\s+#.*)?\s*$`)
			for _, ex := range examples {
				if !line.MatchString(ex.block) {
					t.Errorf("example for %s has no `type: %s` line", ct, ct)
				}
			}
		})
	}
}

// Every documented example must be a check the loader accepts, with all of
// the type's required fields, so the documentation cannot drift from the schema.
func TestChecksDocsExamplesAreValid(t *testing.T) {
	doc := readEvaluationDoc(t)
	found := documentedChecks(t, doc)
	hidden := filepath.Join("testdata", "evals", "fixtures", "hidden")

	for ct, examples := range found {
		if _, known := checkSpecs[ct]; !known {
			t.Errorf("docs/evaluation.md has an example with unknown check type %q", ct)
			continue
		}
		for i, ex := range examples {
			c := ex.check
			if err := ValidateChecks("docs/evaluation.md", []Check{c}, hidden, nil); err != nil {
				t.Errorf("example #%d for %s is not a valid check: %v", i+1, ct, err)
			}
		}
	}
}

// The Checks section must keep the warnings and rules an author depends on.
func TestChecksDocsStatesKeyRules(t *testing.T) {
	doc := readEvaluationDoc(t)
	for _, want := range []string{
		"## Checks",
		"run on the host",
		"--sandbox",
		"passed out of total",
		"legacy heuristics",
		"Evaluation order",
		"fixtures/hidden/",
		"EvaluateChecks",
		"agent produced no output; checks not run",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/evaluation.md does not contain %q", want)
		}
	}
}
