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
	rubric    string            // "" => criteria alpha, beta
}

const auditDefaultRubric = "agent: demo\ncriteria:\n  - name: alpha\n    description: a\n    scoring: \"1-5\"\n  - name: beta\n    description: b\n    scoring: \"1-5\"\n"

// auditTable renders a load-bearing table from "instruction|criterion" rows
// ("=>" separates instruction from criterion; pipes in the instruction are escaped).
func auditTable(rows ...string) string {
	var b strings.Builder
	b.WriteString("| Instruction | Criterion | Evidence |\n|---|---|---|\n")
	for _, r := range rows {
		ins, crit, _ := strings.Cut(r, "=>")
		fmt.Fprintf(&b, "| %s | %s | e |\n", strings.ReplaceAll(ins, "|", `\|`), crit)
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
			"Instructions: total=5 mapped=5 coverage=100.0% (threshold 90%)",
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
		if !strings.Contains(out, "total=2 mapped=2") {
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
		if !strings.Contains(out, "total=2 mapped=2") || strings.Count(out, "skill eng ") != 1 {
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
	if !strings.Contains(out, "total=3 mapped=1 coverage=33.3%") {
		t.Errorf("report missing totals:\n%s", out)
	}
}

func TestAuditCoverageThreshold(t *testing.T) {
	t.Run("fixture exactly 90 percent passes", func(t *testing.T) {
		out := mustPassAudit(t, auditFixtureOpts("coverage-90"))
		if !strings.Contains(out, "total=10 mapped=9 coverage=90.0%") {
			t.Errorf("report missing totals:\n%s", out)
		}
	})
	t.Run("fixture 89 percent fails", func(t *testing.T) {
		out, ae := runAudit(t, auditFixtureOpts("coverage-89"))
		if ae == nil || !auditHas(ae, "coverage", "89.0%") {
			t.Fatalf("want coverage violation at 89.0%%, got %v", ae)
		}
		if !strings.Contains(out, "total=100 mapped=89 coverage=89.0%") {
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
		if !strings.Contains(out, "total=0 mapped=0") {
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
	t.Run("evidence is not verified", func(t *testing.T) {
		tbl := "| Instruction | Criterion | Evidence |\n|---|---|---|\n| Rule one. | alpha | x |\n| Rule two. | beta | whatever |\n"
		mustPassAudit(t, auditCase{prompt: prompt, table: tbl}.write(t))
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
	if !strings.Contains(out, "total=5 mapped=5 coverage=100.0%") {
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
	if !strings.Contains(out, "total=1 mapped=1") {
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
