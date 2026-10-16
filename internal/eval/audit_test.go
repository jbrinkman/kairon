package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

const auditRTCC = "## Role\n## Task\n## Context\n## Constraints\n"

// auditCase builds a self-contained agent "demo" in a temp dir.
type auditCase struct {
	prompt    string
	table     string // "" => no load-bearing.md
	resources []string
	skills    map[string]string // skill name -> SKILL.md content, served via SkillsFS
	rubric    string            // "" => criteria alpha, beta, gamma
	// caseChecks maps case name -> criteria the case has a check on; it is
	// written to cases/demo/<name>.yaml. nil => one case "basic" with checks on
	// alpha, beta and gamma (each with a distinct check body).
	caseChecks map[string][]string
	noCases    bool              // true => no cases/ directory at all
	rawCases   map[string]string // file name -> raw YAML written to cases/demo/ (for malformed input)
}

const auditDefaultRubric = "agent: demo\ncriteria:\n  - name: alpha\n    description: a\n    scoring: \"1-5\"\n  - name: beta\n    description: b\n    scoring: \"1-5\"\n  - name: gamma\n    description: c\n    scoring: \"1-5\"\n"

// auditTable renders a load-bearing table from "instruction|criterion" rows
// ("=>" separates instruction from criterion; pipes in the instruction are escaped).
func auditTable(rows ...string) string {
	var b strings.Builder
	b.WriteString("| Instruction | Criterion | Evidence |\n|---|---|---|\n")
	for _, r := range rows {
		ins, crit, _ := strings.Cut(r, "=>")
		fmt.Fprintf(&b, "| %s | %s | case:basic |\n", strings.ReplaceAll(ins, "|", `\|`), crit)
	}
	return b.String()
}

func (c auditCase) write(t *testing.T) AuditOptions {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := make([]string, 0, len(c.resources))
	for _, r := range c.resources {
		res = append(res, "skill://.kiro/skills/"+r+"/SKILL.md")
	}
	conf, err := json.Marshal(map[string]any{"name": "demo", "prompt": "file://./demo-prompt.md", "resources": res})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite("evals/agents/demo.json", string(conf))
	mustWrite("evals/agents/demo-prompt.md", c.prompt)
	rubric := c.rubric
	if rubric == "" {
		rubric = auditDefaultRubric
	}
	mustWrite("evals/rubrics/demo.yaml", rubric)
	if c.table != "" {
		mustWrite("iterations/demo/load-bearing.md", c.table)
	}
	caseChecks := c.caseChecks
	if caseChecks == nil {
		caseChecks = map[string][]string{"basic": {"alpha", "beta", "gamma"}}
	}
	if !c.noCases {
		for name, criteria := range caseChecks {
			var b strings.Builder
			fmt.Fprintf(&b, "name: %s\nagent: demo\ninput: x\nchecks:\n", name)
			for _, crit := range criteria {
				fmt.Fprintf(&b, "  - criterion: %s\n    type: changed_files\n    allow: [%s.md]\n", crit, crit)
			}
			mustWrite("evals/cases/demo/"+name+".yaml", b.String())
		}
		for file, content := range c.rawCases {
			mustWrite("evals/cases/demo/"+file, content)
		}
	}
	skills := fstest.MapFS{}
	for name, content := range c.skills {
		skills[name+"/SKILL.md"] = &fstest.MapFile{Data: []byte(content)}
	}
	return AuditOptions{
		EvalsDir:      filepath.Join(root, "evals"),
		IterationsDir: filepath.Join(root, "iterations"),
		SkillsFS:      skills,
	}
}

func auditFixtureOpts(name string) AuditOptions {
	root := filepath.Join("testdata", "audit", name)
	return AuditOptions{
		EvalsDir:      filepath.Join(root, "evals"),
		IterationsDir: filepath.Join(root, "iterations"),
		SkillsFS:      os.DirFS(filepath.Join(root, "skills")),
	}
}

func runAudit(t *testing.T, opts AuditOptions) (string, *AuditError) {
	t.Helper()
	var buf bytes.Buffer
	err := Audit(&buf, "demo", opts)
	if err == nil {
		return buf.String(), nil
	}
	var ae *AuditError
	if !errors.As(err, &ae) {
		t.Fatalf("Audit returned a non-AuditError error: %v", err)
	}
	return buf.String(), ae
}

func mustPassAudit(t *testing.T, opts AuditOptions) string {
	t.Helper()
	out, ae := runAudit(t, opts)
	if ae != nil {
		t.Fatalf("Audit failed, want nil:\n%s\noutput:\n%s", ae.Error(), out)
	}
	return out
}

// rules returns the rule IDs of ae's violations (with duplicates).
func auditRules(ae *AuditError) []string {
	var r []string
	for _, v := range ae.Violations {
		r = append(r, v.Rule)
	}
	return r
}

func auditHas(ae *AuditError, rule, substr string) bool {
	for _, v := range ae.Violations {
		if v.Rule == rule && strings.Contains(v.Message, substr) {
			return true
		}
	}
	return false
}

func TestAuditExtractInstructions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []auditInstruction
	}{
		{"blank lines skipped", "\n   \n\t\nreal\n", []auditInstruction{{"f", 4, "real"}}},
		{"all heading levels skipped", "# a\n## b\n### c\n#### d\n##### e\n###### f\nbody\n", []auditInstruction{{"f", 7, "body"}}},
		{"empty heading skipped", "#\n##\nbody", []auditInstruction{{"f", 3, "body"}}},
		{"hash without space is an instruction", "#hashtag\n", []auditInstruction{{"f", 1, "#hashtag"}}},
		{"seven hashes is an instruction", "####### x\n", []auditInstruction{{"f", 1, "####### x"}}},
		{"four space indent is not a heading", "    # code-ish\n", []auditInstruction{{"f", 1, "# code-ish"}}},
		{"table separator skipped", "| a | b |\n|---|:--:|\n| - | - |\n", []auditInstruction{{"f", 1, "| a | b |"}}},
		{"separator needs a dash and a pipe", "---\n|||\n- - -\n", []auditInstruction{{"f", 1, "---"}, {"f", 2, "|||"}, {"f", 3, "- - -"}}},
		{"backtick fence skipped", "a\n```go\nx := 1\n```\nb\n", []auditInstruction{{"f", 1, "a"}, {"f", 5, "b"}}},
		{"tilde fence skipped", "a\n~~~\nx\n~~~\nb\n", []auditInstruction{{"f", 1, "a"}, {"f", 5, "b"}}},
		{"longer closer closes", "````\nx\n``````\nafter\n", []auditInstruction{{"f", 4, "after"}}},
		{"shorter closer does not close", "````\nx\n```\nstill\n````\nafter\n", []auditInstruction{{"f", 6, "after"}}},
		{"other char does not close", "```\nx\n~~~\nstill\n```\nafter\n", []auditInstruction{{"f", 6, "after"}}},
		{"unclosed fence runs to EOF", "a\n```\nx\ny\n", []auditInstruction{{"f", 1, "a"}}},
		{"inline backtick span is not a fence opener", "```foo```\nb\n", []auditInstruction{{"f", 1, "```foo```"}, {"f", 2, "b"}}},
		{"backtick opener with backtick in info string is not a fence", "```go `x`\ninside\n", []auditInstruction{{"f", 1, "```go `x`"}, {"f", 2, "inside"}}},
		{"indented fence", "  ```\nx\n  ```\nafter\n", []auditInstruction{{"f", 4, "after"}}},
		{"front-matter skipped", "---\nname: x\ndescription: y\n---\nbody\n", []auditInstruction{{"f", 5, "body"}}},
		{"unclosed front-matter is ordinary lines", "---\nname: x\nbody\n", []auditInstruction{{"f", 1, "---"}, {"f", 2, "name: x"}, {"f", 3, "body"}}},
		{"mid-file rule counted", "a\n---\nb\n", []auditInstruction{{"f", 1, "a"}, {"f", 2, "---"}, {"f", 3, "b"}}},
		{"front-matter only at the very start", "\n---\nx: 1\n---\nb\n", []auditInstruction{{"f", 2, "---"}, {"f", 3, "x: 1"}, {"f", 4, "---"}, {"f", 5, "b"}}},
		{"each occurrence counts", "same\nsame\nsame\n", []auditInstruction{{"f", 1, "same"}, {"f", 2, "same"}, {"f", 3, "same"}}},
		{"CRLF and trimming", "  a  \r\n\r\nb\r\n", []auditInstruction{{"f", 1, "a"}, {"f", 3, "b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInstructions("f", tt.text)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractInstructions(%q)\n got  %+v\n want %+v", tt.text, got, tt.want)
			}
		})
	}
}

func TestAuditStructure(t *testing.T) {
	// Every line is mapped so only structure can fail.
	mk := func(prompt string) AuditOptions {
		var rows []string
		for _, in := range extractInstructions("p", prompt) {
			rows = append(rows, in.Text+"=>alpha")
		}
		return auditCase{prompt: prompt, table: auditTable(rows...)}.write(t)
	}

	t.Run("all present in order passes", func(t *testing.T) {
		out := mustPassAudit(t, mk("## Role\nr\n## Task\nt\n## Context\nc\n## Constraints\nk\n"))
		if !strings.Contains(out, "RTCC structure present") {
			t.Errorf("success line missing RTCC note:\n%s", out)
		}
	})
	t.Run("extra headings allowed between", func(t *testing.T) {
		mustPassAudit(t, mk("# Title\n## Role\nr\n### Sub\n## Extra\ne\n## Task\nt\n## Context\nc\n## Constraints\nk\n"))
	})
	t.Run("closed ATX headings", func(t *testing.T) {
		mustPassAudit(t, mk("## Role ##\nr\n## Task\n## Context\n## Constraints\n"))
	})
	t.Run("no headings names all four", func(t *testing.T) {
		_, ae := runAudit(t, mk("just text\n"))
		if ae == nil {
			t.Fatal("want structure violations")
		}
		for _, h := range []string{"Role", "Task", "Context", "Constraints"} {
			if !auditHas(ae, "structure", `missing required heading "## `+h+`"`) {
				t.Errorf("no structure violation naming ## %s:\n%s", h, ae.Error())
			}
		}
	})
	t.Run("partial names only the missing", func(t *testing.T) {
		_, ae := runAudit(t, mk("## Role\n## Task\n"))
		if ae == nil {
			t.Fatal("want violations")
		}
		if !auditHas(ae, "structure", `"## Context"`) || !auditHas(ae, "structure", `"## Constraints"`) {
			t.Errorf("missing Context/Constraints not reported:\n%s", ae.Error())
		}
		if auditHas(ae, "structure", `"## Role"`) || auditHas(ae, "structure", `"## Task"`) {
			t.Errorf("present headings reported as missing:\n%s", ae.Error())
		}
	})
	t.Run("out of order reported", func(t *testing.T) {
		_, ae := runAudit(t, mk("## Role\n## Context\n## Task\n## Constraints\n"))
		if ae == nil {
			t.Fatal("want out-of-order violation")
		}
		if !auditHas(ae, "structure", "out of order: want Role, Task, Context, Constraints; found Role, Context, Task, Constraints") {
			t.Errorf("order violation missing:\n%s", ae.Error())
		}
		if auditHas(ae, "structure", "missing") {
			t.Errorf("unexpected missing violation:\n%s", ae.Error())
		}
	})
	t.Run("out of order and missing both reported", func(t *testing.T) {
		_, ae := runAudit(t, mk("## Task\n## Role\n"))
		if ae == nil || !auditHas(ae, "structure", "out of order") || !auditHas(ae, "structure", `"## Context"`) {
			t.Fatalf("want both missing and out-of-order, got %v", ae)
		}
	})
	t.Run("headings inside fences do not count", func(t *testing.T) {
		_, ae := runAudit(t, mk("## Role\n## Task\n```\n## Context\n## Constraints\n```\n~~~\n## Context\n~~~\n"))
		if ae == nil || !auditHas(ae, "structure", `"## Context"`) || !auditHas(ae, "structure", `"## Constraints"`) {
			t.Fatalf("fenced headings were counted: %v", ae)
		}
	})
	t.Run("wrong level or case does not count", func(t *testing.T) {
		_, ae := runAudit(t, mk("### Role\n## role\n# Task\n## Context\n## Constraints\n"))
		if ae == nil || !auditHas(ae, "structure", `"## Role"`) || !auditHas(ae, "structure", `"## Task"`) {
			t.Fatalf("wrong-level/case headings were accepted: %v", ae)
		}
	})
}

func TestAuditFixtureStructure(t *testing.T) {
	_, ae := runAudit(t, auditFixtureOpts("no-rtcc"))
	if ae == nil {
		t.Fatal("no-rtcc must fail")
	}
	for _, h := range []string{"Role", "Task", "Context", "Constraints"} {
		if !auditHas(ae, "structure", `"## `+h+`"`) {
			t.Errorf("no-rtcc: ## %s not named:\n%s", h, ae.Error())
		}
	}
	if auditHas(ae, "coverage", "") || auditHas(ae, "unmapped", "") {
		t.Errorf("no-rtcc isolates structure but got:\n%s", ae.Error())
	}

	_, ae = runAudit(t, auditFixtureOpts("wrong-order"))
	if ae == nil || !auditHas(ae, "structure", "out of order") || len(ae.Violations) != 1 {
		t.Errorf("wrong-order: want exactly one order violation, got %v", ae)
	}

	_, ae = runAudit(t, auditFixtureOpts("partial"))
	if ae == nil || len(ae.Violations) != 2 || !auditHas(ae, "structure", `"## Context"`) || !auditHas(ae, "structure", `"## Constraints"`) {
		t.Errorf("partial: want Context+Constraints missing only, got %v", ae)
	}
}

func TestAuditResources(t *testing.T) {
	prompt := auditRTCC + "Prompt rule.\n"
	table := auditTable("Prompt rule.=>alpha", "Engine rule.=>beta")

	t.Run("fixture: hooks excluded, engine skill audited", func(t *testing.T) {
		out := mustPassAudit(t, auditFixtureOpts("ok-hooks"))
		for _, want := range []string{
			"Audited resources:", "skill engine-skill (1 instructions)",
			"Override hooks (excluded from score):", "skill demo-conventions",
			"Instructions: total=5 mapped=5 duplicates=0 coverage=100.0% (threshold 90%)",
			"OK demo: 5 instructions, 100.0% load-bearing, RTCC structure present",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("report missing %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "Junk line") {
			t.Errorf("hook content leaked into the report:\n%s", out)
		}
	})

	t.Run("hook is never read", func(t *testing.T) {
		// No SKILL.md for the hook exists in SkillsFS; it must not matter.
		mustPassAudit(t, auditCase{prompt: prompt, table: auditTable("Prompt rule.=>alpha"), resources: []string{"demo-conventions"}}.write(t))
	})

	t.Run("engine skill lines count in the totals", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: table, resources: []string{"eng"}, skills: map[string]string{"eng": "---\nname: eng\n---\nEngine rule.\n"}}.write(t)
		out := mustPassAudit(t, opts)
		if !strings.Contains(out, "total=2 mapped=2 duplicates=0") {
			t.Errorf("want total=2 mapped=2:\n%s", out)
		}
	})

	t.Run("unshipped skill is a resource violation", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: auditTable("Prompt rule.=>alpha"), resources: []string{"ghost-skill"}}.write(t)
		_, ae := runAudit(t, opts)
		if ae == nil || !auditHas(ae, "resource", `skill "ghost-skill" is not shipped`) {
			t.Fatalf("want resource violation, got %v", ae)
		}
	})

	t.Run("nil SkillsFS is a resource violation", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: auditTable("Prompt rule.=>alpha"), resources: []string{"eng"}}.write(t)
		opts.SkillsFS = nil
		_, ae := runAudit(t, opts)
		if ae == nil || !auditHas(ae, "resource", `skill "eng"`) {
			t.Fatalf("want resource violation, got %v", ae)
		}
	})

	t.Run("duplicates processed once, other resources ignored", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: table, resources: []string{"eng", "eng"}, skills: map[string]string{"eng": "Engine rule.\n"}}.write(t)
		conf := filepath.Join(opts.EvalsDir, "agents", "demo.json")
		data, err := os.ReadFile(conf)
		if err != nil {
			t.Fatal(err)
		}
		patched := strings.Replace(string(data), `"resources":[`, `"resources":["file://x.md",42,`, 1)
		if patched == string(data) {
			t.Fatalf("test setup: could not patch %s", data)
		}
		if err := os.WriteFile(conf, []byte(patched), 0o644); err != nil {
			t.Fatal(err)
		}
		out := mustPassAudit(t, opts)
		if !strings.Contains(out, "total=2 mapped=2 duplicates=0") || strings.Count(out, "skill eng ") != 1 {
			t.Errorf("duplicate skill counted twice:\n%s", out)
		}
		if !strings.Contains(out, "Ignored resources:") || !strings.Contains(out, "file://x.md") {
			t.Errorf("ignored resources not listed:\n%s", out)
		}
	})
}

func TestAuditUnmappedFixture(t *testing.T) {
	opts := auditFixtureOpts("unmapped")
	out, ae := runAudit(t, opts)
	if ae == nil {
		t.Fatalf("unmapped fixture must fail; output:\n%s", out)
	}
	prompt := filepath.Join(opts.EvalsDir, "agents", "demo-prompt.md")
	rubric := filepath.Join(opts.EvalsDir, "rubrics", "demo.yaml")
	wantUnmapped := fmt.Sprintf(`%s:5: unmapped: instruction not in load-bearing table: "Orphan line."`, prompt)
	wantUnknown := fmt.Sprintf(`%s:8: unknown-criterion: maps to criterion "ghost" which is not in rubric %s: "Ghost line."`, prompt, rubric)
	got := ae.Error()
	for _, want := range []string{wantUnmapped, wantUnknown} {
		if !strings.Contains(got, want) {
			t.Errorf("error missing line %q\n got:\n%s", want, got)
		}
	}
	if !strings.Contains(out, "total=3 mapped=1 duplicates=0 coverage=33.3%") {
		t.Errorf("report missing totals:\n%s", out)
	}
}

func TestAuditCoverageThreshold(t *testing.T) {
	t.Run("fixture exactly 90 percent passes", func(t *testing.T) {
		out := mustPassAudit(t, auditFixtureOpts("coverage-90"))
		if !strings.Contains(out, "total=10 mapped=9 duplicates=0 coverage=90.0%") {
			t.Errorf("report missing totals:\n%s", out)
		}
	})
	t.Run("fixture 89 percent fails", func(t *testing.T) {
		out, ae := runAudit(t, auditFixtureOpts("coverage-89"))
		if ae == nil || !auditHas(ae, "coverage", "89.0%") {
			t.Fatalf("want coverage violation at 89.0%%, got %v", ae)
		}
		if !strings.Contains(out, "total=100 mapped=89 duplicates=0 coverage=89.0%") {
			t.Errorf("report missing totals:\n%s", out)
		}
	})

	gen := func(total, mapped int) AuditOptions {
		var p strings.Builder
		p.WriteString(auditRTCC)
		var rows []string
		for i := 0; i < total; i++ {
			fmt.Fprintf(&p, "Rule %04d.\n", i)
			if i < mapped {
				rows = append(rows, fmt.Sprintf("Rule %04d.=>alpha", i))
			}
		}
		return auditCase{prompt: p.String(), table: auditTable(rows...)}.write(t)
	}
	t.Run("899 of 1000 displays 89.9 and fails", func(t *testing.T) {
		out, ae := runAudit(t, gen(1000, 899))
		if ae == nil || !auditHas(ae, "coverage", "89.9%") {
			t.Fatalf("want coverage violation at 89.9%%, got %v", ae)
		}
		if !strings.Contains(out, "coverage=89.9%") {
			t.Errorf("report must display 89.9%%:\n%s", out)
		}
	})
	t.Run("900 of 1000 passes", func(t *testing.T) {
		mustPassAudit(t, gen(1000, 900))
	})
	t.Run("zero instructions fails closed", func(t *testing.T) {
		out, ae := runAudit(t, auditCase{prompt: auditRTCC, table: auditTable("x=>alpha")}.write(t))
		if ae == nil || !auditHas(ae, "coverage", "") {
			t.Fatalf("want coverage violation, got %v", ae)
		}
		if !strings.Contains(out, "total=0 mapped=0 duplicates=0") {
			t.Errorf("report missing totals:\n%s", out)
		}
	})
}

func TestAuditTable(t *testing.T) {
	prompt := auditRTCC + "Rule one.\nRule two.\n"

	t.Run("missing table reports table and unmapped", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt}.write(t))
		if ae == nil || !auditHas(ae, "table", "load-bearing table not found") || !auditHas(ae, "unmapped", `"Rule one."`) {
			t.Fatalf("got %v", ae)
		}
	})
	t.Run("no header", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt, table: "| Rule | Crit | E |\n|---|---|---|\n| Rule one. | alpha | e |\n"}.write(t))
		if ae == nil || !auditHas(ae, "table", `no "| Instruction | Criterion | Evidence |" header row`) {
			t.Fatalf("got %v", ae)
		}
	})
	t.Run("wrong column count is reported and skipped", func(t *testing.T) {
		tbl := auditTable("Rule one.=>alpha") + "| Rule two. | alpha |\n| a | b | c | d |\n"
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		if ae == nil || !auditHas(ae, "table", "expected 3 columns, got 2") || !auditHas(ae, "table", "expected 3 columns, got 4") {
			t.Fatalf("got %v", ae)
		}
		if !auditHas(ae, "unmapped", `"Rule two."`) {
			t.Errorf("skipped row must leave its instruction unmapped: %v", ae)
		}
	})
	t.Run("empty cells", func(t *testing.T) {
		tbl := auditTable("Rule one.=>alpha") + "|  | alpha | e |\n| Rule two. |  | e |\n"
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		if ae == nil || strings.Count(ae.Error(), ": table: ") < 2 {
			t.Fatalf("want two table violations, got %v", ae)
		}
	})
	t.Run("conflicting duplicate rows", func(t *testing.T) {
		tbl := auditTable("Rule one.=>alpha", "Rule one.=>beta", "Rule two.=>alpha")
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		if ae == nil || !auditHas(ae, "table", "Rule one.") {
			t.Fatalf("got %v", ae)
		}
	})
	t.Run("identical duplicate rows are fine", func(t *testing.T) {
		tbl := auditTable("Rule one.=>alpha", "Rule one.=>alpha", "Rule two.=>alpha")
		mustPassAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
	})
	t.Run("rows before the header and in fences are ignored", func(t *testing.T) {
		tbl := "| Rule one. | alpha | e |\n```\n| Rule two. | alpha | e |\n```\n\n" + auditTable("Rule one.=>alpha", "Rule two.=>alpha")
		mustPassAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
	})
	t.Run("evidence is verified", func(t *testing.T) {
		tbl := "| Instruction | Criterion | Evidence |\n|---|---|---|\n| Rule one. | alpha | x |\n| Rule two. | beta | whatever |\n"
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		if ae == nil || strings.Count(ae.Error(), ": table: ") != 2 {
			t.Fatalf("want two table violations for evidence without case:<name>, got %v", ae)
		}
	})
	t.Run("rubric criterion name is exact", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt, table: auditTable("Rule one.=>Alpha", "Rule two.=>alpha")}.write(t))
		if ae == nil || !auditHas(ae, "unknown-criterion", `"Alpha"`) {
			t.Fatalf("got %v", ae)
		}
	})
	t.Run("matching is verbatim after trimming", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: auditRTCC + "  Rule one.  \nRule  two.\n", table: auditTable("Rule one.=>alpha", "Rule two.=>alpha")}.write(t))
		if ae == nil || !auditHas(ae, "unmapped", `"Rule  two."`) || auditHas(ae, "unmapped", `"Rule one."`) {
			t.Fatalf("got %v", ae)
		}
	})
}

func TestAuditFenceAndTableFixture(t *testing.T) {
	out := mustPassAudit(t, auditFixtureOpts("fence-and-table"))
	if !strings.Contains(out, "total=5 mapped=5 duplicates=0 coverage=100.0%") {
		t.Errorf("want 5 instructions (front-matter, fences, headings and separator skipped):\n%s", out)
	}
}

func TestAuditCandidate(t *testing.T) {
	opts := auditFixtureOpts("candidate")

	_, ae := runAudit(t, opts)
	if ae == nil || !auditHas(ae, "structure", `"## Role"`) {
		t.Fatalf("live prompt is unstructured and must fail: %v", ae)
	}

	opts.PromptFile = filepath.Join("testdata", "audit", "candidate", "candidate.md")
	out := mustPassAudit(t, opts)
	if !strings.Contains(out, "(candidate)") || !strings.Contains(out, "candidate.md") || strings.Contains(out, "Live line.") {
		t.Errorf("report should name the candidate:\n%s", out)
	}
	if !strings.Contains(out, "total=1 mapped=1 duplicates=0") {
		t.Errorf("only the candidate's lines should be counted:\n%s", out)
	}

	// The candidate's lines are checked against the *agent's* table.
	other := auditCase{prompt: "live\n", table: auditTable("something else=>alpha")}.write(t)
	other.PromptFile = opts.PromptFile
	if _, ae := runAudit(t, other); ae == nil || !auditHas(ae, "unmapped", "Candidate line.") {
		t.Errorf("candidate must be mapped against the agent's table: %v", ae)
	}

	t.Run("empty candidate is a plain error", func(t *testing.T) {
		empty := filepath.Join(t.TempDir(), "empty.md")
		if err := os.WriteFile(empty, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		o := auditFixtureOpts("candidate")
		o.PromptFile = empty
		assertPlainAuditError(t, o, "empty")
	})
	t.Run("directory candidate is a plain error", func(t *testing.T) {
		o := auditFixtureOpts("candidate")
		o.PromptFile = t.TempDir()
		assertPlainAuditError(t, o, "not a regular file")
	})
	t.Run("missing candidate is a plain error", func(t *testing.T) {
		o := auditFixtureOpts("candidate")
		o.PromptFile = filepath.Join(t.TempDir(), "nope.md")
		assertPlainAuditError(t, o, "nope.md")
	})
}

func assertPlainAuditError(t *testing.T, opts AuditOptions, substr string) {
	t.Helper()
	var buf bytes.Buffer
	err := Audit(&buf, "demo", opts)
	if err == nil {
		t.Fatal("want an error")
	}
	var ae *AuditError
	if errors.As(err, &ae) {
		t.Fatalf("want a plain error, got AuditError: %v", err)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err, substr)
	}
}

func TestAuditPlainErrors(t *testing.T) {
	good := auditCase{prompt: auditRTCC + "x\n", table: auditTable("x=>alpha")}

	t.Run("bad agent name", func(t *testing.T) {
		for _, name := range []string{"", "../x", "a/b"} {
			if err := Audit(&bytes.Buffer{}, name, good.write(t)); err == nil {
				t.Errorf("agent %q: want error", name)
			}
		}
	})
	t.Run("missing agent config", func(t *testing.T) {
		o := good.write(t)
		var buf bytes.Buffer
		err := Audit(&buf, "nobody", o)
		if err == nil || !strings.Contains(err.Error(), "nobody") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("missing rubric", func(t *testing.T) {
		o := good.write(t)
		if err := os.Remove(filepath.Join(o.EvalsDir, "rubrics", "demo.yaml")); err != nil {
			t.Fatal(err)
		}
		assertPlainAuditError(t, o, "rubric")
	})
	t.Run("rubric without criteria", func(t *testing.T) {
		c := good
		c.rubric = "agent: demo\ncriteria: []\n"
		assertPlainAuditError(t, c.write(t), "criteria")
	})
	t.Run("inline prompt", func(t *testing.T) {
		o := good.write(t)
		conf := filepath.Join(o.EvalsDir, "agents", "demo.json")
		if err := os.WriteFile(conf, []byte(`{"name":"demo","prompt":"inline text"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		assertPlainAuditError(t, o, "inline")
	})
}

func TestAuditErrorFormat(t *testing.T) {
	e := &AuditError{Violations: []AuditViolation{
		{File: "a.md", Line: 3, Rule: "unmapped", Message: "m1"},
		{File: "b.md", Rule: "table", Message: "m2"},
	}}
	if got, want := e.Error(), "a.md:3: unmapped: m1\nb.md: table: m2"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAuditOrderingIsStable(t *testing.T) {
	_, ae := runAudit(t, auditFixtureOpts("unmapped"))
	if ae == nil {
		t.Fatal("want violations")
	}
	// prompt violations by line, then the coverage rule last.
	got := auditRules(ae)
	want := []string{"unmapped", "unknown-criterion", "coverage"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rule order = %v, want %v", got, want)
	}
}

// auditFlatFixtureOpts returns options for a flat-layout fixture where the
// iterations dir and the evals dir are the same directory (agent "selftest").
func auditFlatFixtureOpts(name string) AuditOptions {
	root := filepath.Join("testdata", "audit", name)
	return AuditOptions{EvalsDir: root, IterationsDir: root}
}

func runAuditAgent(t *testing.T, agent string, opts AuditOptions) (string, *AuditError) {
	t.Helper()
	var buf bytes.Buffer
	err := Audit(&buf, agent, opts)
	if err == nil {
		return buf.String(), nil
	}
	var ae *AuditError
	if !errors.As(err, &ae) {
		t.Fatalf("Audit returned a non-AuditError error: %v", err)
	}
	return buf.String(), ae
}

// tableViolations returns the "table" violations of ae.
func tableViolations(ae *AuditError) []AuditViolation {
	var out []AuditViolation
	if ae == nil {
		return nil
	}
	for _, v := range ae.Violations {
		if v.Rule == auditRuleTable {
			out = append(out, v)
		}
	}
	return out
}

func TestAuditEvidence(t *testing.T) {
	prompt := auditRTCC + "Rule one.\n"
	// row renders a one-row table; the data row is on line 3.
	row := func(criterion, evidence string) string {
		return fmt.Sprintf("| Instruction | Criterion | Evidence |\n|---|---|---|\n| Rule one. | %s | %s |\n", criterion, evidence)
	}
	// wantOne asserts exactly one table violation at line wantLine whose message contains every substr.
	wantOne := func(t *testing.T, c auditCase, wantLine int, substrs ...string) {
		t.Helper()
		_, ae := runAudit(t, c.write(t))
		tv := tableViolations(ae)
		if len(tv) != 1 {
			t.Fatalf("want exactly one table violation, got %d (err: %v)", len(tv), ae)
		}
		if tv[0].Line != wantLine {
			t.Errorf("violation line = %d, want %d: %s", tv[0].Line, wantLine, tv[0])
		}
		for _, s := range substrs {
			if !strings.Contains(tv[0].Message, s) {
				t.Errorf("message %q does not contain %q", tv[0].Message, s)
			}
		}
	}
	wantPass := func(t *testing.T, c auditCase) {
		t.Helper()
		mustPassAudit(t, c.write(t))
	}

	t.Run("empty evidence", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "")}, 3,
			"row 3:", `Evidence must name at least one case of agent "demo" as case:<case-name>`)
	})
	t.Run("text without token", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "covered by the basic case")}, 3,
			`Evidence must name at least one case of agent "demo"`)
	})
	t.Run("showcase: is not a token", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "showcase:basic")}, 3,
			"Evidence must name at least one case")
	})
	t.Run("underscore and digit before case: is not a boundary", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "x_case:basic 1case:basic")}, 3,
			"Evidence must name at least one case")
	})
	t.Run("bare case: with empty name is not a token", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:. case:-")}, 3,
			"Evidence must name at least one case")
	})
	t.Run("token resolves", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "case:basic")})
	})
	t.Run("trailing dot is sentence punctuation", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "Proved by case:basic.")})
	})
	t.Run("trailing dash is stripped", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "case:basic-")})
	})
	t.Run("backticks and surrounding text", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "see `case:basic`, which checks it")})
	})
	t.Run("dotted and dashed case names", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "case:my-case.v2"),
			caseChecks: map[string][]string{"my-case.v2": {"alpha"}, "rest": {"beta", "gamma"}}})
	})
	t.Run("case name is matched by name, not file name", func(t *testing.T) {
		c := auditCase{prompt: prompt, table: row("alpha", "case:renamed"), noCases: false,
			rawCases: map[string]string{"other-file.yaml": "name: renamed\nchecks:\n  - criterion: alpha\n    type: changed_files\n"}}
		wantPass(t, c)
	})
	t.Run("case names are case-sensitive", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:Basic")}, 3, `"Basic"`, "not a case")
	})
	t.Run("unknown case names line and case", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:ghost")}, 3,
			"row 3:", `evidence names case "ghost" which is not a case of agent "demo"`,
			filepath.Join("cases", "demo"))
	})
	t.Run("case without a check on the criterion", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("beta", "case:only-alpha"),
			caseChecks: map[string][]string{"only-alpha": {"alpha"}}}, 3,
			"row 3:", `case "only-alpha" has no check on criterion "beta"`)
	})
	t.Run("case with no checks at all", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:bare"),
			caseChecks: map[string][]string{"bare": nil}}, 3, `case "bare" has no check on criterion "alpha"`)
	})
	t.Run("unknown criterion does not also report a missing check", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt, table: row("ghost", "case:basic")}.write(t))
		if ae == nil {
			t.Fatal("want unknown-criterion failure")
		}
		if tv := tableViolations(ae); len(tv) != 0 {
			t.Errorf("unknown criterion must not yield table violations, got %v", tv)
		}
	})
	t.Run("unknown criterion still reports a missing token and an unknown case", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt, table: row("ghost", "nothing")}.write(t))
		if len(tableViolations(ae)) != 1 {
			t.Errorf("missing token must still be reported: %v", ae)
		}
		_, ae = runAudit(t, auditCase{prompt: prompt, table: row("ghost", "case:nope")}.write(t))
		if len(tableViolations(ae)) != 1 || !auditHas(ae, "table", `"nope"`) {
			t.Errorf("unknown case must still be reported: %v", ae)
		}
	})
	t.Run("several tokens, one bad, yields one violation naming it", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:basic and case:ghost, case:basic")}, 3, `"ghost"`)
	})
	t.Run("several bad tokens each reported once", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt, table: row("alpha", "case:g1 case:g2 case:g1")}.write(t))
		tv := tableViolations(ae)
		if len(tv) != 2 || !strings.Contains(tv[0].Message, `"g1"`) || !strings.Contains(tv[1].Message, `"g2"`) {
			t.Fatalf("want one violation each for g1 and g2 (in order), got %v", tv)
		}
	})
	t.Run("several named cases all checking the criterion pass", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("beta", "case:b-one case:b-too"),
			caseChecks: map[string][]string{"b-one": {"beta"}, "b-too": {"alpha", "beta"}, "g-case": {"gamma"}}})
	})
	t.Run("every named case must check the criterion", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("beta", "case:a-only case:b-too"),
			caseChecks: map[string][]string{"a-only": {"alpha"}, "b-too": {"alpha", "beta"}}}, 3, `case "a-only" has no check on criterion "beta"`)
	})
	t.Run("repeated identical row with bad evidence is reported on its own line", func(t *testing.T) {
		tbl := "| Instruction | Criterion | Evidence |\n|---|---|---|\n| Rule one. | alpha | case:basic |\n| Rule one. | alpha | case:ghost |\n"
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		tv := tableViolations(ae)
		if len(tv) != 1 || tv[0].Line != 4 || !strings.Contains(tv[0].Message, `"ghost"`) {
			t.Fatalf("want one violation on line 4 naming ghost, got %v", tv)
		}
	})
	t.Run("conflicting duplicate row still has its evidence validated", func(t *testing.T) {
		tbl := "| Instruction | Criterion | Evidence |\n|---|---|---|\n| Rule one. | alpha | case:basic |\n| Rule one. | beta | nothing |\n"
		_, ae := runAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
		if !auditHas(ae, "table", "already mapped") || !auditHas(ae, "table", "Evidence must name at least one case") {
			t.Fatalf("want both the conflict and the evidence violation, got %v", ae)
		}
	})
	t.Run("rows with empty instruction or criterion get no evidence violation", func(t *testing.T) {
		tbl := auditTable("Rule one.=>alpha") + "|  | alpha |  |\n| Rule two. |  |  |\n"
		_, ae := runAudit(t, auditCase{prompt: auditRTCC + "Rule one.\nRule two.\n", table: tbl}.write(t))
		if tv := tableViolations(ae); len(tv) != 2 || auditHas(ae, "table", "Evidence must name") {
			t.Fatalf("want only the two empty-cell violations, got %v", tv)
		}
	})
	t.Run("violations carry the table path and rule", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: row("alpha", "")}.write(t)
		_, ae := runAudit(t, opts)
		tv := tableViolations(ae)
		if len(tv) != 1 {
			t.Fatalf("got %v", ae)
		}
		wantFile := filepath.Join(opts.IterationsDir, "demo", "load-bearing.md")
		if tv[0].File != wantFile || tv[0].Rule != "table" {
			t.Errorf("violation = %+v, want file %s rule table", tv[0], wantFile)
		}
		if want := fmt.Sprintf("%s:3: table: row 3: Evidence must name", wantFile); !strings.Contains(ae.Error(), want) {
			t.Errorf("rendered error missing %q:\n%s", want, ae.Error())
		}
	})
	t.Run("reported even when coverage is above the threshold", func(t *testing.T) {
		// 10/10 instructions are mapped (100% coverage) but one row has bad evidence.
		var p strings.Builder
		p.WriteString(auditRTCC)
		var tbl strings.Builder
		tbl.WriteString("| Instruction | Criterion | Evidence |\n|---|---|---|\n")
		for i := 0; i < 10; i++ {
			fmt.Fprintf(&p, "Rule %02d.\n", i)
			ev := "case:basic"
			if i == 4 {
				ev = "case:ghost"
			}
			fmt.Fprintf(&tbl, "| Rule %02d. | alpha | %s |\n", i, ev)
		}
		out, ae := runAudit(t, auditCase{prompt: p.String(), table: tbl.String()}.write(t))
		if ae == nil {
			t.Fatalf("evidence violation must fail the audit at 100%% coverage; output:\n%s", out)
		}
		if !strings.Contains(out, "coverage=100.0%") {
			t.Errorf("setup: coverage should be 100%%:\n%s", out)
		}
		if tv := tableViolations(ae); len(tv) != 1 || tv[0].Line != 7 {
			t.Errorf("want one table violation at line 7, got %v", tv)
		}
		if auditHas(ae, "coverage", "") {
			t.Errorf("no coverage violation expected: %v", ae)
		}
	})
	t.Run("missing cases dir means no cases", func(t *testing.T) {
		wantOne(t, auditCase{prompt: prompt, table: row("alpha", "case:basic"), noCases: true}, 3,
			`evidence names case "basic" which is not a case of agent "demo"`)
	})
	t.Run("non-yaml files and subdirectories in cases are ignored", func(t *testing.T) {
		opts := auditCase{prompt: prompt, table: row("alpha", "case:basic")}.write(t)
		dir := filepath.Join(opts.EvalsDir, "cases", "demo")
		if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(": : not yaml"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sub", "ghost.yaml"), []byte("name: ghost\nchecks:\n  - criterion: alpha\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustPassAudit(t, opts)
	})
	t.Run("malformed case YAML is a plain error", func(t *testing.T) {
		assertPlainAuditError(t, auditCase{prompt: prompt, table: row("alpha", "case:basic"),
			rawCases: map[string]string{"broken.yaml": "name: [unterminated\nchecks: {"}}.write(t), "broken.yaml")
	})
	t.Run("case file without a name is a plain error", func(t *testing.T) {
		assertPlainAuditError(t, auditCase{prompt: prompt, table: row("alpha", "case:basic"),
			rawCases: map[string]string{"nameless.yaml": "agent: demo\nchecks:\n  - criterion: alpha\n"}}.write(t), "nameless.yaml")
	})
	t.Run("same name in two files merges criteria", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("beta", "case:dup"),
			caseChecks: map[string][]string{"dup": {"alpha"}, "g-case": {"gamma"}},
			rawCases:   map[string]string{"dup-b.yaml": "name: dup\nchecks:\n  - criterion: beta\n    type: changed_files\n"}})
	})
	t.Run("unknown case fields are ignored", func(t *testing.T) {
		wantPass(t, auditCase{prompt: prompt, table: row("alpha", "case:basic"), caseChecks: map[string][]string{"rest": {"beta", "gamma"}},
			rawCases: map[string]string{"x.yaml": "name: basic\nsomething_new: 1\nchecks:\n  - criterion: alpha\n    type: made_up\n    extra: true\n"}})
	})
	t.Run("missing table reports no evidence violations", func(t *testing.T) {
		_, ae := runAudit(t, auditCase{prompt: prompt}.write(t))
		if auditHas(ae, "table", "Evidence") {
			t.Errorf("no table, no evidence validation: %v", ae)
		}
	})
}

func TestAuditEvidenceFixtures(t *testing.T) {
	type want struct {
		fixture string
		table   []string // substrings, each must be in a distinct table violation
	}
	tests := []want{
		{"evidence-missing", []string{
			"3: table: row 3: Evidence must name at least one case of agent \"selftest\" as case:<case-name>",
			"4: table: row 4: Evidence must name at least one case of agent \"selftest\" as case:<case-name>",
		}},
		{"evidence-unknown-case", []string{`4: table: row 4: evidence names case "ghost" which is not a case of agent "selftest"`}},
		{"evidence-unchecked-criterion", []string{`5: table: row 5: case "alpha-only" has no check on criterion "beta"`}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			opts := auditFlatFixtureOpts(tt.fixture)
			out, ae := runAuditAgent(t, "selftest", opts)
			if ae == nil {
				t.Fatalf("%s must fail; output:\n%s", tt.fixture, out)
			}
			tablePath := filepath.Join(opts.IterationsDir, "selftest", "load-bearing.md")
			got := ae.Error()
			for _, w := range tt.table {
				if want := tablePath + ":" + w; !strings.Contains(got, want) {
					t.Errorf("error missing %q\n got:\n%s", want, got)
				}
			}
			if n := len(tableViolations(ae)); n != len(tt.table) || len(ae.Violations) != n {
				t.Errorf("want exactly %d violations, all table; got %d table of %d:\n%s", len(tt.table), n, len(ae.Violations), got)
			}
			if !strings.Contains(out, "coverage=100.0%") {
				t.Errorf("fixture isolates evidence (full coverage):\n%s", out)
			}
		})
	}
}

func TestAuditDuplicates(t *testing.T) {
	const skillFile = "cmd/kairon/templates/kiro/skills/eng/SKILL.md"
	promptOf := func(opts AuditOptions) string { return filepath.Join(opts.EvalsDir, "agents", "demo-prompt.md") }
	// dupViolations returns the "duplicate" violations of ae.
	dupViolations := func(ae *AuditError) []AuditViolation {
		var out []AuditViolation
		if ae == nil {
			return nil
		}
		for _, v := range ae.Violations {
			if v.Rule == "duplicate" {
				out = append(out, v)
			}
		}
		return out
	}

	t.Run("repeat in the prompt names both lines", func(t *testing.T) {
		// auditRTCC is 4 lines, so the rules are on lines 5, 6 and 7.
		opts := auditCase{
			prompt: auditRTCC + "Rule one.\nRule two.\nRule one.\n",
			table:  auditTable("Rule one.=>alpha", "Rule two.=>beta"),
		}.write(t)
		out, ae := runAudit(t, opts)
		dv := dupViolations(ae)
		if len(dv) != 1 {
			t.Fatalf("want exactly one duplicate violation, got %d (err: %v)", len(dv), ae)
		}
		p := promptOf(opts)
		if dv[0].File != p || dv[0].Line != 7 {
			t.Errorf("violation at %s:%d, want %s:7", dv[0].File, dv[0].Line, p)
		}
		if want := fmt.Sprintf(`duplicate of %s:5: "Rule one."`, p); dv[0].Message != want {
			t.Errorf("message = %q, want %q", dv[0].Message, want)
		}
		if want := fmt.Sprintf(`%s:7: duplicate: duplicate of %s:5: "Rule one."`, p, p); !strings.Contains(ae.Error(), want) {
			t.Errorf("error missing %q\n got:\n%s", want, ae.Error())
		}
		// The repeat is still counted, and fully mapped coverage does not hide it.
		if !strings.Contains(out, "Instructions: total=3 mapped=3 duplicates=1 coverage=100.0% (threshold 90%)") {
			t.Errorf("report line wrong:\n%s", out)
		}
		if strings.Contains(out, "OK demo") {
			t.Errorf("duplicates must prevent the OK line:\n%s", out)
		}
	})

	t.Run("third occurrence names the first, not the second", func(t *testing.T) {
		opts := auditCase{
			prompt: auditRTCC + "Rule one.\nRule one.\nRule one.\n",
			table:  auditTable("Rule one.=>alpha"),
		}.write(t)
		out, ae := runAudit(t, opts)
		dv := dupViolations(ae)
		if len(dv) != 2 {
			t.Fatalf("want two duplicate violations, got %d (err: %v)", len(dv), ae)
		}
		p := promptOf(opts)
		for i, line := range []int{6, 7} {
			if dv[i].Line != line {
				t.Errorf("violation %d at line %d, want %d", i, dv[i].Line, line)
			}
			if want := fmt.Sprintf("duplicate of %s:5:", p); !strings.Contains(dv[i].Message, want) {
				t.Errorf("violation %d message %q must name the first occurrence (%s)", i, dv[i].Message, want)
			}
		}
		if !strings.Contains(out, "total=3 mapped=3 duplicates=2 coverage=100.0%") {
			t.Errorf("report line wrong:\n%s", out)
		}
	})

	t.Run("comparison uses the trimmed text", func(t *testing.T) {
		opts := auditCase{
			prompt: auditRTCC + "Rule one.\n   Rule one.\t\n",
			table:  auditTable("Rule one.=>alpha"),
		}.write(t)
		_, ae := runAudit(t, opts)
		if len(dupViolations(ae)) != 1 {
			t.Errorf("want one duplicate for the indented repeat, got %v", ae)
		}
	})

	t.Run("engine skill repeating a prompt line", func(t *testing.T) {
		opts := auditCase{
			prompt:    auditRTCC + "Rule one.\n",
			table:     auditTable("Rule one.=>alpha"),
			resources: []string{"eng"},
			skills:    map[string]string{"eng": "---\nname: eng\n---\nRule one.\n"},
		}.write(t)
		out, ae := runAudit(t, opts)
		dv := dupViolations(ae)
		if len(dv) != 1 {
			t.Fatalf("want one duplicate violation, got %d (err: %v)", len(dv), ae)
		}
		if dv[0].File != skillFile || dv[0].Line != 4 {
			t.Errorf("violation at %s:%d, want %s:4", dv[0].File, dv[0].Line, skillFile)
		}
		if want := fmt.Sprintf(`duplicate of %s:5: "Rule one."`, promptOf(opts)); dv[0].Message != want {
			t.Errorf("message = %q, want %q", dv[0].Message, want)
		}
		if !strings.Contains(out, "total=2 mapped=2 duplicates=1") {
			t.Errorf("report line wrong:\n%s", out)
		}
	})

	t.Run("fenced code and headings are never duplicates", func(t *testing.T) {
		prompt := auditRTCC + "Rule one.\n```\nRule one.\nRule one.\n```\n## Task\n## Task\n### Rule one.\n"
		opts := auditCase{prompt: prompt, table: auditTable("Rule one.=>alpha")}.write(t)
		_, ae := runAudit(t, opts)
		if dv := dupViolations(ae); len(dv) != 0 {
			t.Errorf("fences and headings must not be duplicates: %v", dv)
		}
	})

	t.Run("distinct lines are not duplicates", func(t *testing.T) {
		opts := auditCase{
			prompt: auditRTCC + "Rule one.\nRule two.\n",
			table:  auditTable("Rule one.=>alpha", "Rule two.=>beta"),
		}.write(t)
		out := mustPassAudit(t, opts)
		if !strings.Contains(out, "duplicates=0") {
			t.Errorf("want duplicates=0:\n%s", out)
		}
	})

	t.Run("reported even when coverage is far above the threshold", func(t *testing.T) {
		// 20 distinct mapped rules plus one repeat: 21/21 mapped.
		var p strings.Builder
		p.WriteString(auditRTCC)
		var rows []string
		for i := 0; i < 20; i++ {
			fmt.Fprintf(&p, "Rule %02d.\n", i)
			rows = append(rows, fmt.Sprintf("Rule %02d.=>alpha", i))
		}
		p.WriteString("Rule 00.\n")
		_, ae := runAudit(t, auditCase{prompt: p.String(), table: auditTable(rows...)}.write(t))
		if ae == nil || len(dupViolations(ae)) != 1 {
			t.Fatalf("want exactly one duplicate violation, got %v", ae)
		}
		if got := auditRules(ae); !reflect.DeepEqual(got, []string{"duplicate"}) {
			t.Errorf("rules = %v, want only [duplicate]", got)
		}
	})

	t.Run("repeat of an unmapped line is a duplicate and counted unmapped", func(t *testing.T) {
		opts := auditCase{
			prompt: auditRTCC + "Mapped.\nOrphan.\nOrphan.\n",
			table:  auditTable("Mapped.=>alpha"),
		}.write(t)
		out, ae := runAudit(t, opts)
		if ae == nil || len(dupViolations(ae)) != 1 {
			t.Fatalf("want one duplicate violation, got %v", ae)
		}
		if !strings.Contains(out, "total=3 mapped=1 duplicates=1 coverage=33.3%") {
			t.Errorf("counting must be unchanged:\n%s", out)
		}
	})
}

func TestAuditDuplicateRuleOrder(t *testing.T) {
	// Same file and line: table < duplicate < unmapped.
	rank, ok := auditRuleOrder["duplicate"]
	if !ok || !(auditRuleOrder[auditRuleTable] < rank && rank < auditRuleOrder[auditRuleUnmapped]) {
		t.Errorf("duplicate must rank after table and before unmapped: %v", auditRuleOrder)
	}
	// An unmapped repeat sorts duplicate before unmapped on the same line.
	_, ae := runAudit(t, auditCase{prompt: auditRTCC + "Orphan.\nOrphan.\n", table: auditTable("Other.=>alpha")}.write(t))
	if ae == nil {
		t.Fatal("want violations")
	}
	var got []string
	for _, v := range ae.Violations {
		if v.Line == 6 {
			got = append(got, v.Rule)
		}
	}
	if want := []string{"duplicate", "unmapped"}; !reflect.DeepEqual(got, want) {
		t.Errorf("line 6 rules = %v, want %v", got, want)
	}
}

func TestAuditReportDuplicates(t *testing.T) {
	t.Run("clean prompt prints duplicates=0", func(t *testing.T) {
		out := mustPassAudit(t, auditCase{prompt: auditRTCC + "Rule one.\n", table: auditTable("Rule one.=>alpha")}.write(t))
		want := "Instructions: total=1 mapped=1 duplicates=0 coverage=100.0% (threshold 90%)\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	})
	t.Run("exact line with repeats", func(t *testing.T) {
		out, _ := runAudit(t, auditCase{prompt: auditRTCC + "A.\nA.\nA.\nB.\n", table: auditTable("A.=>alpha")}.write(t))
		want := "Instructions: total=4 mapped=3 duplicates=2 coverage=75.0% (threshold 90%)\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	})
}

func TestAuditDuplicateLineFixture(t *testing.T) {
	opts := auditFlatFixtureOpts("duplicate-line")
	out, ae := runAuditAgent(t, "selftest", opts)
	if ae == nil {
		t.Fatalf("duplicate-line must fail; output:\n%s", out)
	}
	prompt := filepath.Join(opts.EvalsDir, "agents", "selftest-prompt.md")
	want := fmt.Sprintf(`%s:9: duplicate: duplicate of %s:8: "Never skip validation."`, prompt, prompt)
	if !strings.Contains(ae.Error(), want) {
		t.Errorf("error missing %q\n got:\n%s", want, ae.Error())
	}
	if len(ae.Violations) != 1 {
		t.Errorf("want only the duplicate violation, got:\n%s", ae.Error())
	}
	if !strings.Contains(out, "Instructions: total=5 mapped=5 duplicates=1 coverage=100.0% (threshold 90%)") {
		t.Errorf("report must print duplicates=1:\n%s", out)
	}
}

// auditRubricOf renders a rubric for agent "demo". Each entry is a criterion
// name, or "name:cost" for a criterion of type cost.
func auditRubricOf(entries ...string) string {
	var b strings.Builder
	b.WriteString("agent: demo\ncriteria:\n")
	for _, e := range entries {
		name, typ, _ := strings.Cut(e, ":")
		fmt.Fprintf(&b, "  - name: %s\n    description: d\n", name)
		if typ == "cost" {
			b.WriteString("    type: cost\n")
		} else {
			b.WriteString("    scoring: \"1-5\"\n")
		}
	}
	return b.String()
}

// auditRubricPath is where auditCase.write puts the rubric.
func auditRubricPath(opts AuditOptions) string {
	return filepath.Join(opts.EvalsDir, "rubrics", "demo.yaml")
}

// auditRuleViolations returns the violations of ae with the given rule.
func auditRuleViolations(ae *AuditError, rule string) []AuditViolation {
	var out []AuditViolation
	if ae == nil {
		return nil
	}
	for _, v := range ae.Violations {
		if v.Rule == rule {
			out = append(out, v)
		}
	}
	return out
}

// auditRawCase renders a case file whose checks are given verbatim (each entry
// is the YAML of one list item, without the leading "- ").
func auditRawCase(name string, checks ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\nagent: demo\ninput: x\nchecks:\n", name)
	for _, c := range checks {
		lines := strings.Split(strings.TrimRight(c, "\n"), "\n")
		b.WriteString("  - " + lines[0] + "\n")
		for _, l := range lines[1:] {
			b.WriteString("    " + l + "\n")
		}
	}
	return b.String()
}

// auditCriteriaCase is a passing-by-default agent whose cases are given raw.
func auditCriteriaCase(rubric string, rawCases map[string]string) auditCase {
	return auditCase{
		prompt:     auditRTCC + "Rule one.\n",
		table:      auditTable("Rule one.=>alpha"),
		rubric:     rubric,
		caseChecks: map[string][]string{}, // only rawCases
		rawCases:   rawCases,
	}
}

func TestAuditRubricMinimumCriteria(t *testing.T) {
	prompt := auditRTCC + "Rule one.\n"
	table := auditTable("Rule one.=>alpha")
	run := func(t *testing.T, rubric string) (AuditOptions, *AuditError) {
		t.Helper()
		opts := auditCase{prompt: prompt, table: table, rubric: rubric}.write(t)
		_, ae := runAudit(t, opts)
		return opts, ae
	}
	wantRubric := func(t *testing.T, rubric string, substrs ...string) {
		t.Helper()
		opts, ae := run(t, rubric)
		vs := auditRuleViolations(ae, auditRuleRubric)
		if len(vs) != 1 {
			t.Fatalf("want exactly one rubric violation, got %d (err: %v)", len(vs), ae)
		}
		if vs[0].File != auditRubricPath(opts) || vs[0].Line != 0 {
			t.Errorf("violation = %s:%d, want %s with no line", vs[0].File, vs[0].Line, auditRubricPath(opts))
		}
		for _, s := range substrs {
			if !strings.Contains(vs[0].Message, s) {
				t.Errorf("message %q does not contain %q", vs[0].Message, s)
			}
		}
	}
	wantNone := func(t *testing.T, rubric string) {
		t.Helper()
		_, ae := run(t, rubric)
		if vs := auditRuleViolations(ae, auditRuleRubric); len(vs) != 0 {
			t.Errorf("want no rubric violation, got %v", vs)
		}
	}

	t.Run("two non-cost", func(t *testing.T) {
		wantRubric(t, auditRubricOf("alpha", "beta"), "has 2 ", "alpha", "beta", "at least 3")
	})
	t.Run("one non-cost", func(t *testing.T) {
		wantRubric(t, auditRubricOf("alpha"), "has 1 ", "alpha", "at least 3")
	})
	t.Run("cost criterion does not count", func(t *testing.T) {
		wantRubric(t, auditRubricOf("alpha", "beta", "cost:cost"), "has 2 ", "alpha", "beta", "at least 3")
	})
	t.Run("only cost", func(t *testing.T) {
		wantRubric(t, auditRubricOf("cost:cost"), "has 0 ", "at least 3")
	})
	t.Run("duplicate names count once", func(t *testing.T) {
		wantRubric(t, auditRubricOf("alpha", "beta", "alpha"), "has 2 ")
	})
	t.Run("three non-cost", func(t *testing.T) {
		wantNone(t, auditRubricOf("alpha", "beta", "gamma"))
	})
	t.Run("three non-cost plus cost", func(t *testing.T) {
		wantNone(t, auditRubricOf("alpha", "beta", "gamma", "cost:cost"))
	})
	t.Run("more than three", func(t *testing.T) {
		wantNone(t, auditRubricOf("alpha", "beta", "gamma", "delta"))
	})
	t.Run("reported even at full coverage", func(t *testing.T) {
		out, ae := runAudit(t, auditCase{prompt: prompt, table: table, rubric: auditRubricOf("alpha", "beta")}.write(t))
		if ae == nil || len(auditRuleViolations(ae, auditRuleRubric)) != 1 {
			t.Fatalf("want a rubric violation, got %v", ae)
		}
		if !strings.Contains(out, "coverage=100.0%") {
			t.Errorf("coverage must be unaffected:\n%s", out)
		}
	})
}

func TestAuditUncheckedCriterion(t *testing.T) {
	prompt := auditRTCC + "Rule one.\n"
	table := auditTable("Rule one.=>alpha")
	run := func(t *testing.T, c auditCase) (AuditOptions, *AuditError) {
		t.Helper()
		c.prompt, c.table = prompt, table
		opts := c.write(t)
		_, ae := runAudit(t, opts)
		return opts, ae
	}
	// unchecked returns the criterion name quoted in each unchecked violation.
	unchecked := func(opts AuditOptions, ae *AuditError) []string {
		var out []string
		for _, v := range auditRuleViolations(ae, auditRuleUnchecked) {
			if v.File != auditRubricPath(opts) || v.Line != 0 {
				t.Errorf("violation = %s:%d, want %s with no line", v.File, v.Line, auditRubricPath(opts))
			}
			for _, n := range []string{"alpha", "beta", "gamma", "cost"} {
				if strings.Contains(v.Message, `"`+n+`"`) {
					out = append(out, n)
				}
			}
		}
		return out
	}

	t.Run("criterion without any check", func(t *testing.T) {
		opts, ae := run(t, auditCase{caseChecks: map[string][]string{"basic": {"alpha", "beta"}}})
		if got := unchecked(opts, ae); !reflect.DeepEqual(got, []string{"gamma"}) {
			t.Fatalf("unchecked = %v, want [gamma] (err: %v)", got, ae)
		}
		v := auditRuleViolations(ae, auditRuleUnchecked)[0]
		if dir := filepath.Join(opts.EvalsDir, "cases", "demo"); !strings.Contains(v.Message, dir) {
			t.Errorf("message %q does not name the cases directory %q", v.Message, dir)
		}
	})
	t.Run("checked only in the second of two cases", func(t *testing.T) {
		opts, ae := run(t, auditCase{caseChecks: map[string][]string{"basic": {"alpha", "beta"}, "second": {"gamma"}}})
		if got := unchecked(opts, ae); len(got) != 0 {
			t.Errorf("unchecked = %v, want none", got)
		}
	})
	t.Run("no cases directory", func(t *testing.T) {
		opts, ae := run(t, auditCase{noCases: true})
		if got := unchecked(opts, ae); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
			t.Errorf("unchecked = %v, want [alpha beta gamma]", got)
		}
	})
	t.Run("checks on unknown criteria satisfy nothing", func(t *testing.T) {
		opts, ae := run(t, auditCase{caseChecks: map[string][]string{"basic": {"alpha", "beta", "zeta"}}})
		if got := unchecked(opts, ae); !reflect.DeepEqual(got, []string{"gamma"}) {
			t.Errorf("unchecked = %v, want [gamma]", got)
		}
	})
	t.Run("cost criterion without checks is not reported", func(t *testing.T) {
		opts, ae := run(t, auditCase{rubric: auditRubricOf("alpha", "beta", "gamma", "cost:cost")})
		if got := unchecked(opts, ae); len(got) != 0 {
			t.Errorf("unchecked = %v, want none", got)
		}
		if ae != nil {
			t.Errorf("want the audit to pass, got %v", ae)
		}
	})
	t.Run("cost criterion with checks is not reported", func(t *testing.T) {
		opts, ae := run(t, auditCase{
			rubric:     auditRubricOf("alpha", "beta", "gamma", "cost:cost"),
			caseChecks: map[string][]string{"basic": {"alpha", "beta", "gamma", "cost"}},
		})
		if got := unchecked(opts, ae); len(got) != 0 {
			t.Errorf("unchecked = %v, want none", got)
		}
	})
	t.Run("padded criterion name matched by a padded check is not unchecked", func(t *testing.T) {
		// A rubric name and a case check with the same surrounding spaces must
		// be treated as the same criterion: both sides are trimmed, so the
		// check counts and the criterion is not reported unchecked.
		rubric := "agent: demo\ncriteria:\n" +
			"  - name: alpha\n    scoring: \"1-5\"\n" +
			"  - name: beta\n    scoring: \"1-5\"\n" +
			"  - name: \" gamma \"\n    scoring: \"1-5\"\n"
		cases := map[string]string{
			"basic.yaml": auditRawCase("basic",
				"criterion: alpha\ntype: changed_files\nallow: [alpha.md]",
				"criterion: beta\ntype: changed_files\nallow: [beta.md]",
				"criterion: \" gamma \"\ntype: changed_files\nallow: [gamma.md]"),
		}
		opts := auditCase{
			prompt:     auditRTCC + "Rule one.\n",
			table:      auditTable("Rule one.=>alpha"),
			rubric:     rubric,
			caseChecks: map[string][]string{},
			rawCases:   cases,
		}.write(t)
		_, ae := runAudit(t, opts)
		if got := unchecked(opts, ae); len(got) != 0 {
			t.Fatalf("unchecked = %v, want none (padded name should match padded check); err: %v", got, ae)
		}
	})
}

func TestAuditIdenticalCriteria(t *testing.T) {
	const (
		alpha = "criterion: alpha\ntype: changed_files\nallow: [alpha.md]"
		x     = "type: changed_files\nallow: [x.md]"
		y     = "type: changed_files\nallow: [y.md]"
	)
	crit := func(name, body string) string { return "criterion: " + name + "\n" + body }
	run := func(t *testing.T, rubric string, cases map[string]string) (AuditOptions, *AuditError) {
		t.Helper()
		opts := auditCriteriaCase(rubric, cases).write(t)
		_, ae := runAudit(t, opts)
		return opts, ae
	}
	// wantPairs asserts the identical violations: each entry is "first,repeat"
	// and the message must name both, in quotes.
	wantPairs := func(t *testing.T, opts AuditOptions, ae *AuditError, pairs ...[2]string) {
		t.Helper()
		vs := auditRuleViolations(ae, auditRuleIdentical)
		if len(vs) != len(pairs) {
			t.Fatalf("want %d identical violations, got %d (err: %v)", len(pairs), len(vs), ae)
		}
		for i, p := range pairs {
			v := vs[i]
			if v.File != auditRubricPath(opts) || v.Line != 0 {
				t.Errorf("violation = %s:%d, want %s with no line", v.File, v.Line, auditRubricPath(opts))
			}
			for _, n := range p {
				if !strings.Contains(v.Message, `"`+n+`"`) {
					t.Errorf("message %q does not name %q", v.Message, n)
				}
			}
		}
	}
	wantNone := func(t *testing.T, ae *AuditError) {
		t.Helper()
		if vs := auditRuleViolations(ae, auditRuleIdentical); len(vs) != 0 {
			t.Errorf("want no identical violation, got %v", vs)
		}
	}
	rubric := auditRubricOf("alpha", "beta", "gamma")

	t.Run("same body in the only case", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha, crit("beta", x), crit("gamma", x))})
		wantPairs(t, opts, ae, [2]string{"beta", "gamma"})
	})
	t.Run("field order does not matter", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			"criterion: beta\ntype: changed_files\nallow: [x.md]",
			"allow: [x.md]\ncriterion: gamma\ntype: changed_files")})
		wantPairs(t, opts, ae, [2]string{"beta", "gamma"})
	})
	t.Run("flow versus quoted list is the same", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			"criterion: beta\ntype: changed_files\nallow: [x.md]",
			"criterion: gamma\ntype: changed_files\nallow:\n  - \"x.md\"")})
		wantPairs(t, opts, ae, [2]string{"beta", "gamma"})
	})
	t.Run("check order within a case does not matter", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			crit("beta", x), crit("beta", y), crit("gamma", y), crit("gamma", x))})
		wantPairs(t, opts, ae, [2]string{"beta", "gamma"})
	})
	t.Run("different value", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha, crit("beta", x), crit("gamma", y))})
		wantNone(t, ae)
	})
	t.Run("different type", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			crit("beta", x), crit("gamma", "type: file_exists\nallow: [x.md]"))})
		wantNone(t, ae)
	})
	t.Run("extra field", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			crit("beta", x), crit("gamma", x+"\nextra: 1"))})
		wantNone(t, ae)
	})
	t.Run("same in one case, different in another", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{
			"basic.yaml":  auditRawCase("basic", alpha, crit("beta", x), crit("gamma", x)),
			"second.yaml": auditRawCase("second", crit("beta", x), crit("gamma", y)),
		})
		wantNone(t, ae)
	})
	t.Run("same in every case", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{
			"basic.yaml":  auditRawCase("basic", alpha, crit("beta", x), crit("gamma", x)),
			"second.yaml": auditRawCase("second", crit("beta", y), crit("gamma", y)),
		})
		wantPairs(t, opts, ae, [2]string{"beta", "gamma"})
	})
	t.Run("checked in a subset of the cases", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{
			"basic.yaml":  auditRawCase("basic", alpha, crit("beta", x), crit("gamma", x)),
			"second.yaml": auditRawCase("second", crit("beta", x)),
		})
		wantNone(t, ae)
	})
	t.Run("check counts differ", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha,
			crit("beta", x), crit("gamma", x), crit("gamma", x))})
		wantNone(t, ae)
	})
	t.Run("three identical", func(t *testing.T) {
		opts, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", crit("alpha", x), crit("beta", x), crit("gamma", x))})
		wantPairs(t, opts, ae, [2]string{"alpha", "beta"}, [2]string{"alpha", "gamma"})
	})
	t.Run("criteria without checks are unchecked, not identical", func(t *testing.T) {
		_, ae := run(t, rubric, map[string]string{"basic.yaml": auditRawCase("basic", alpha)})
		wantNone(t, ae)
		if n := len(auditRuleViolations(ae, auditRuleUnchecked)); n != 2 {
			t.Errorf("want 2 unchecked violations (beta, gamma), got %d: %v", n, ae)
		}
	})
	t.Run("cost criterion is never identical", func(t *testing.T) {
		_, ae := run(t, auditRubricOf("alpha", "beta", "gamma", "cost:cost"), map[string]string{"basic.yaml": auditRawCase("basic",
			crit("alpha", x), crit("beta", y), crit("gamma", "type: file_exists\nallow: [z.md]"), crit("cost", x))})
		wantNone(t, ae)
	})
}

// TestCanonicalAuditCheckUnencodable pins that a check body JSON cannot encode
// is reported as an error rather than silently collapsed to a lossy fmt %v
// string (which drops value types and string boundaries, so two different
// checks could canonicalize identically and wrongly trigger the identical rule).
func TestCanonicalAuditCheckUnencodable(t *testing.T) {
	// A channel is not JSON-encodable, so json.Marshal fails on this body.
	body := map[string]any{"bad": make(chan int)}
	if _, err := canonicalAuditCheck(body); err == nil {
		t.Fatal("canonicalAuditCheck returned no error for a JSON-unencodable body; it must not fall back to a lossy format")
	}

	ok := map[string]any{"type": "changed_files", "allow": []any{"a.md"}}
	got, err := canonicalAuditCheck(ok)
	if err != nil {
		t.Fatalf("canonicalAuditCheck on an encodable body: unexpected error %v", err)
	}
	if want := `{"allow":["a.md"],"type":"changed_files"}`; got != want {
		t.Fatalf("canonicalAuditCheck = %q, want %q", got, want)
	}
}

func TestAuditReportCriteria(t *testing.T) {
	prompt := auditRTCC + "Rule one.\n"
	table := auditTable("Rule one.=>alpha")
	t.Run("pass lists every non-cost criterion in rubric order", func(t *testing.T) {
		out := mustPassAudit(t, auditCase{
			prompt: prompt, table: table,
			rubric:     auditRubricOf("alpha", "beta", "cost:cost", "gamma"),
			caseChecks: map[string][]string{"basic": {"alpha", "alpha", "beta", "gamma"}, "second": {"alpha"}},
		}.write(t))
		want := "Criteria: non-cost=3 (need at least 3)\n  alpha: checks=3 cases=2\n  beta: checks=1 cases=1\n  gamma: checks=1 cases=1\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
		if strings.Contains(out, "cost:") && strings.Contains(out, "  cost") {
			t.Errorf("cost criterion must not be listed:\n%s", out)
		}
		if i, j := strings.Index(out, "Instructions:"), strings.Index(out, "Criteria:"); i < 0 || j < i {
			t.Errorf("Criteria block must follow the Instructions line:\n%s", out)
		}
	})
	t.Run("rubric order, not alphabetical", func(t *testing.T) {
		out := mustPassAudit(t, auditCase{prompt: prompt, table: table, rubric: auditRubricOf("gamma", "alpha", "beta")}.write(t))
		want := "  gamma: checks=1 cases=1\n  alpha: checks=1 cases=1\n  beta: checks=1 cases=1\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	})
	t.Run("printed on failure too", func(t *testing.T) {
		out, ae := runAudit(t, auditCase{prompt: prompt, table: table, rubric: auditRubricOf("alpha", "beta", "cost:cost")}.write(t))
		if ae == nil {
			t.Fatal("want a rubric violation")
		}
		want := "Criteria: non-cost=2 (need at least 3)\n  alpha: checks=1 cases=1\n  beta: checks=1 cases=1\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	})
	t.Run("unchecked criterion shows zero counts", func(t *testing.T) {
		out, _ := runAudit(t, auditCase{prompt: prompt, table: table, noCases: true}.write(t))
		want := "  gamma: checks=0 cases=0\n"
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	})
	t.Run("no non-cost criteria", func(t *testing.T) {
		out, _ := runAudit(t, auditCase{prompt: prompt, table: table, rubric: auditRubricOf("cost:cost")}.write(t))
		if !strings.Contains(out, "Criteria: non-cost=0 (need at least 3)\n") {
			t.Errorf("report missing the Criteria header:\n%s", out)
		}
	})
}

func TestAuditCriteriaFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		rule    string   // "" => the audit must pass
		substrs []string // message substrings of the single violation
		report  string   // must be in the report
	}{
		{"two-criteria", auditRuleRubric, []string{"has 2 ", "alpha", "beta", "at least 3"},
			"Criteria: non-cost=2 (need at least 3)\n  alpha: checks=1 cases=1\n  beta: checks=1 cases=1\n"},
		{"unchecked-criterion", auditRuleUnchecked, []string{`"gamma"`},
			"  gamma: checks=0 cases=0\n"},
		{"identical-criteria", auditRuleIdentical, []string{`"beta"`, `"gamma"`},
			"  beta: checks=1 cases=1\n  gamma: checks=1 cases=1\n"},
		{"three-criteria-ok", "", nil,
			"Criteria: non-cost=3 (need at least 3)\n  alpha: checks=3 cases=2\n  beta: checks=1 cases=1\n  gamma: checks=1 cases=1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			opts := auditFlatFixtureOpts(tt.fixture)
			out, ae := runAuditAgent(t, "selftest", opts)
			if !strings.Contains(out, tt.report) {
				t.Errorf("report missing %q:\n%s", tt.report, out)
			}
			if !strings.Contains(out, "coverage=100.0%") {
				t.Errorf("fixture isolates the criteria rules (full coverage):\n%s", out)
			}
			if tt.rule == "" {
				if ae != nil {
					t.Fatalf("%s must pass, got:\n%s", tt.fixture, ae.Error())
				}
				return
			}
			if ae == nil {
				t.Fatalf("%s must fail; output:\n%s", tt.fixture, out)
			}
			if len(ae.Violations) != 1 || ae.Violations[0].Rule != tt.rule {
				t.Fatalf("want exactly one %s violation, got:\n%s", tt.rule, ae.Error())
			}
			v := ae.Violations[0]
			if want := filepath.Join(opts.EvalsDir, "rubrics", "selftest.yaml"); v.File != want || v.Line != 0 {
				t.Errorf("violation = %s:%d, want %s with no line", v.File, v.Line, want)
			}
			for _, s := range tt.substrs {
				if !strings.Contains(v.Message, s) {
					t.Errorf("message %q does not contain %q", v.Message, s)
				}
			}
		})
	}
}
