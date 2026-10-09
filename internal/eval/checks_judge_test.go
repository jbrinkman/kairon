package eval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeJudge records every query and answers with a fixed verdict/error.
type fakeJudge struct {
	verdict JudgeVerdict
	err     error
	queries []JudgeQuery
}

func (f *fakeJudge) judge(q JudgeQuery) (JudgeVerdict, error) {
	f.queries = append(f.queries, q)
	return f.verdict, f.err
}

func judgeCheck(question string, files ...string) Check {
	return Check{Criterion: "crit", Type: CheckJudge, Question: question, Files: files}
}

func TestJudgeCheckVocabulary(t *testing.T) {
	if CheckJudge != "judge" {
		t.Errorf("CheckJudge = %q, want judge", CheckJudge)
	}
	spec, ok := checkSpecs[CheckJudge]
	if !ok {
		t.Fatal("judge is not in checkSpecs")
	}
	if !spec.required.question || spec.required.files || !spec.allowed.files || !spec.allowed.question {
		t.Errorf("judge spec wrong: %+v", spec)
	}
}

func TestChecksValidateJudge(t *testing.T) {
	tests := []struct {
		name    string
		check   Check
		wantErr string // "" = valid
	}{
		{"question only", judgeCheck("Is it good?"), ""},
		{"question and files", judgeCheck("Is it good?", "a.md", "dir/b.md"), ""},
		{".eval file allowed", judgeCheck("Is it good?", ".eval/issue-body.md"), ""},
		{"empty files list", Check{Criterion: "clarity", Type: CheckJudge, Question: "q", Files: []string{}}, ""},
		{"missing question", Check{Criterion: "clarity", Type: CheckJudge}, "question is required"},
		{"blank question", judgeCheck("   \t"), "question"},
		{"files without question", Check{Criterion: "clarity", Type: CheckJudge, Files: []string{"a"}}, "question is required"},
		{"absolute file", judgeCheck("q", "/etc/passwd"), "files"},
		{"dotdot file", judgeCheck("q", "../x"), "files"},
		{"embedded dotdot file", judgeCheck("q", "a/../../x"), "files"},
		{"empty file entry", judgeCheck("q", ""), "files"},
		{".git file", judgeCheck("q", ".git/config"), ".git"},
		{"nested .git file", judgeCheck("q", "sub/.git/config"), ".git"},
		{"duplicate file", judgeCheck("q", "a.md", "b.md", "a.md"), "duplicate"},
		{"duplicate after clean", judgeCheck("q", "a.md", "./a.md"), "duplicate"},
		{"question on file_exists", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", Question: "q"}, "question is not valid"},
		{"files on file_exists", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", Files: []string{"a"}}, "files is not valid"},
		{"question on output_contains", Check{Criterion: "clarity", Type: CheckOutputContains, Pattern: "x", Question: "q"}, "question is not valid"},
		{"files on command", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Files: []string{"a"}}, "files is not valid"},
		{"run on judge", Check{Criterion: "clarity", Type: CheckJudge, Question: "q", Run: "true"}, "run is not valid"},
		{"path on judge", Check{Criterion: "clarity", Type: CheckJudge, Question: "q", Path: "a"}, "path is not valid"},
		{"pattern on judge", Check{Criterion: "clarity", Type: CheckJudge, Question: "q", Pattern: "a"}, "pattern is not valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check.Criterion = "clarity"
			// A valid check first so the reported index is 2.
			checks := []Check{{Criterion: "clarity", Type: CheckFileExists, Path: "ok"}, tt.check}
			err := ValidateChecks("case-file.yaml", checks, "", nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !errors.Is(err, errInvalidChecks) {
				t.Errorf("error does not wrap errInvalidChecks: %v", err)
			}
			msg := err.Error()
			for _, want := range []string{"case-file.yaml", "#2", string(tt.check.Type), tt.wantErr} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

func TestLoadCasesJudgeCheck(t *testing.T) {
	checksEnv(t)
	writeChecksCase(t, "chk", "judge.yaml", `name: judge
input: x
checks:
  - criterion: clarity
    type: judge
    question: "Is the issue body testable?"
    files: [.eval/issue-body.md, notes.md]
`)
	got, err := loadCases("chk")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Checks) != 1 {
		t.Fatalf("loaded %+v", got)
	}
	c := got[0].Checks[0]
	if c.Type != CheckJudge || c.Question != "Is the issue body testable?" || len(c.Files) != 2 || c.Files[0] != ".eval/issue-body.md" {
		t.Errorf("judge check decoded wrong: %+v", c)
	}
}

func TestLoadCasesJudgeCheckRejected(t *testing.T) {
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"missing question", "  - criterion: clarity\n    type: judge\n", "question is required"},
		{"duplicate file", "  - criterion: clarity\n    type: judge\n    question: q\n    files: [a, a]\n", "duplicate"},
		{"files on other type", "  - criterion: clarity\n    type: file_exists\n    path: a\n    files: [a]\n", "files is not valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksEnv(t)
			writeChecksCase(t, "chk", "bad-case.yaml", "name: bad\ninput: x\nchecks:\n"+tt.yaml)
			_, err := loadCases("chk")
			if err == nil || !errors.Is(err, errInvalidChecks) {
				t.Fatalf("want errInvalidChecks, got %v", err)
			}
			for _, want := range []string{"bad-case.yaml", "#1", tt.wantErr} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestJudgeCheckLabel(t *testing.T) {
	c := judgeCheck("Does the body state a criterion?", "a.md")
	if got, want := checkLabel(2, c), `#2 judge question="Does the body state a criterion?"`; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
	long := judgeCheck(strings.Repeat("q", 200))
	if got := checkLabel(1, long); len(got) > 120 || !strings.HasSuffix(got, `…"`) {
		t.Errorf("long question not shortened: %q", got)
	}
}

func TestJudgeCheckYesPasses(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "clearly stated"}}
	r := evalOne(t, judgeCheck("Is it good?"), CheckInput{Judge: f.judge})
	wantPass(t, r)
	if !strings.Contains(r.Detail, "judge answered yes: clearly stated") {
		t.Errorf("detail %q lacks yes + reasoning", r.Detail)
	}
	if r.Type != CheckJudge || !strings.Contains(r.Label, "judge") {
		t.Errorf("result type/label wrong: %+v", r)
	}
}

func TestJudgeCheckNoFails(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: false, Reasoning: "criterion missing"}}
	r := evalOne(t, judgeCheck("Is it good?"), CheckInput{Judge: f.judge})
	wantFail(t, r, "judge answered no: criterion missing")
}

func TestJudgeCheckEmptyReasoning(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, judgeCheck("q"), CheckInput{Judge: f.judge})
	wantPass(t, r)
	if !strings.Contains(r.Detail, "judge answered yes: (no reasoning given)") {
		t.Errorf("detail %q", r.Detail)
	}
}

func TestJudgeCheckQueryCarriesEverything(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, ".eval/issue-body.md", "body one")
	writeEvalFile(t, dir, "notes/two.md", "body two")
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	c := Check{Criterion: "issue_quality", Type: CheckJudge, Question: "Is it testable?",
		Files: []string{".eval/issue-body.md", "notes/two.md"}}
	r := evalOne(t, c, CheckInput{Dir: dir, Output: "final output", Input: "case input", Judge: f.judge})
	wantPass(t, r)
	if len(f.queries) != 1 {
		t.Fatalf("judge called %d times, want 1", len(f.queries))
	}
	q := f.queries[0]
	if q.Criterion != "issue_quality" || q.Question != "Is it testable?" || q.Input != "case input" || q.Output != "final output" {
		t.Errorf("query wrong: %+v", q)
	}
	if len(q.Files) != 2 {
		t.Fatalf("query has %d files, want 2", len(q.Files))
	}
	if q.Files[0].Path != ".eval/issue-body.md" || q.Files[0].Content != "body one" || q.Files[0].Truncated {
		t.Errorf("file 0 wrong: %+v", q.Files[0])
	}
	if q.Files[1].Path != "notes/two.md" || q.Files[1].Content != "body two" || q.Files[1].Truncated {
		t.Errorf("file 1 wrong: %+v", q.Files[1])
	}
}

func TestJudgeCheckRefusesFilesWithoutCallingJudge(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "real.md", "real")
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"missing", []string{"absent.md"}, "file does not exist"},
		{"missing after a good one", []string{"real.md", "absent.md"}, "file does not exist"},
		{"symlinked file", []string{"link.md"}, "symlink"},
		{"below symlinked dir", []string{"linkdir/secret.md"}, "symlink"},
		{"directory", []string{"adir"}, "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "x"}}
			r := evalOne(t, judgeCheck("q", tt.files...), CheckInput{Dir: dir, Judge: f.judge})
			wantFail(t, r, tt.want)
			if len(f.queries) != 0 {
				t.Errorf("judge was invoked %d times for a refused file", len(f.queries))
			}
		})
	}
}

func TestJudgeCheckFilesNeedWorkspace(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, judgeCheck("q", "a.md"), CheckInput{Judge: f.judge})
	wantFail(t, r, "workspace")
	if len(f.queries) != 0 {
		t.Error("judge invoked without a workspace")
	}
}

func TestJudgeCheckFileCaps(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("a", judgeMaxFileBytes+500)
	writeEvalFile(t, dir, "big.md", big)
	writeEvalFile(t, dir, "small.md", "tiny")
	for i := 0; i < 5; i++ {
		writeEvalFile(t, dir, fmt.Sprintf("f%d.md", i), strings.Repeat("b", judgeMaxFileBytes))
	}

	t.Run("per file", func(t *testing.T) {
		f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
		wantPass(t, evalOne(t, judgeCheck("q", "big.md", "small.md"), CheckInput{Dir: dir, Judge: f.judge}))
		files := f.queries[0].Files
		if !files[0].Truncated || files[0].Size != int64(len(big)) {
			t.Errorf("big file not flagged truncated: truncated=%v size=%d", files[0].Truncated, files[0].Size)
		}
		wantMarker := fmt.Sprintf("[truncated: showing first %d of %d bytes]", judgeMaxFileBytes, len(big))
		if !strings.Contains(files[0].Content, wantMarker) {
			t.Errorf("content lacks marker %q", wantMarker)
		}
		if !strings.HasPrefix(files[0].Content, strings.Repeat("a", judgeMaxFileBytes)) {
			t.Error("content does not start with the first 64 KiB")
		}
		if len(files[0].Content) > judgeMaxFileBytes+200 {
			t.Errorf("content is %d bytes, cap not applied", len(files[0].Content))
		}
		if files[1].Truncated || files[1].Content != "tiny" {
			t.Errorf("small file wrong: %+v", files[1])
		}
	})

	t.Run("total", func(t *testing.T) {
		f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
		wantPass(t, evalOne(t, judgeCheck("q", "f0.md", "f1.md", "f2.md", "f3.md", "f4.md"), CheckInput{Dir: dir, Judge: f.judge}))
		files := f.queries[0].Files
		shown := 0
		for _, jf := range files {
			body := jf.Content
			if i := strings.Index(body, "\n[truncated:"); i >= 0 {
				body = body[:i]
			}
			shown += len(body)
		}
		if shown > judgeMaxTotalBytes {
			t.Errorf("showed %d bytes of file content, want <= %d", shown, judgeMaxTotalBytes)
		}
		for i := 0; i < 4; i++ {
			if files[i].Truncated {
				t.Errorf("file %d (within budget) truncated", i)
			}
		}
		if !files[4].Truncated {
			t.Error("file 4 exceeds the total budget and must be truncated")
		}
		if len(files[4].Content) > 200 || !strings.Contains(files[4].Content, "[truncated: showing first 0 of") {
			t.Errorf("file 4 content = %q", files[4].Content)
		}
	})
}

func TestJudgeCheckTruncatesOnRuneBoundaryAndSanitizesUTF8(t *testing.T) {
	dir := t.TempDir()
	// "é" is 2 bytes, so the 64 KiB cut falls mid-rune for an odd offset.
	writeEvalFile(t, dir, "u.md", "x"+strings.Repeat("é", judgeMaxFileBytes))
	writeEvalFile(t, dir, "bad.md", "ok\xffend")
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	wantPass(t, evalOne(t, judgeCheck("q", "u.md", "bad.md"), CheckInput{Dir: dir, Judge: f.judge}))
	for _, jf := range f.queries[0].Files {
		if !isValidUTF8(jf.Content) {
			t.Errorf("%s content is not valid UTF-8", jf.Path)
		}
	}
	if strings.Contains(f.queries[0].Files[0].Content, "\uFFFD") {
		t.Error("truncation split a rune")
	}
}

func isValidUTF8(s string) bool { return strings.ToValidUTF8(s, "") == s }

func TestJudgeCheckJudgeErrorFails(t *testing.T) {
	f := &fakeJudge{err: errors.New("backend down")}
	r := evalOne(t, judgeCheck("q"), CheckInput{Judge: f.judge})
	wantFail(t, r, "judge call failed: backend down")
}

func TestJudgeCheckParseErrorFails(t *testing.T) {
	f := &fakeJudge{err: fmt.Errorf("%w: JSON delimiters not found", errJudgeParse)}
	r := evalOne(t, judgeCheck("q"), CheckInput{Judge: f.judge})
	wantFail(t, r, "judge parse error: JSON delimiters not found")
	if strings.Contains(r.Detail, "judge call failed") {
		t.Errorf("parse error reported as call failure: %q", r.Detail)
	}
	if strings.Contains(r.Detail, "judge parse error: judge parse error") {
		t.Errorf("parse error prefix doubled: %q", r.Detail)
	}
}

func TestJudgeCheckNilJudgeFails(t *testing.T) {
	r := evalOne(t, judgeCheck("q"), CheckInput{})
	wantFail(t, r, "no judge configured")
}

func TestJudgeCheckNilJudgeFailsEvenWithFiles(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "a.md", "a")
	r := evalOne(t, judgeCheck("q", "a.md"), CheckInput{Dir: dir})
	wantFail(t, r, "no judge configured")
}

// Judge checks run in the non-command pass, in list order, interleaved with
// other non-command checks and before command checks.
func TestJudgeCheckRunsInNonCommandPassInListOrder(t *testing.T) {
	var order []string
	dir := t.TempDir()
	// The command check writes order.txt; the judge must never see it.
	checks := []Check{
		{Criterion: "c", Type: CheckCommand, Run: "echo ran >> order.txt"},
		judgeCheck("first"),
		{Criterion: "c", Type: CheckOutputContains, Pattern: "out"},
		judgeCheck("second"),
	}
	rs := EvaluateChecks(checks, CheckInput{Dir: dir, Output: "out", Judge: func(q JudgeQuery) (JudgeVerdict, error) {
		if _, err := os.Stat(filepath.Join(dir, "order.txt")); err == nil {
			t.Error("a command check ran before a judge check")
		}
		order = append(order, q.Question)
		return JudgeVerdict{Yes: true, Reasoning: "r"}, nil
	}})
	if got := strings.Join(order, ","); got != "first,second" {
		t.Errorf("judge call order = %q, want first,second", got)
	}
	for i, r := range rs {
		if r.Index != i+1 {
			t.Errorf("result %d has index %d", i, r.Index)
		}
		if !r.Passed {
			t.Errorf("result %d failed: %+v", i, r)
		}
	}
	if rs[1].Type != CheckJudge || rs[3].Type != CheckJudge {
		t.Errorf("results out of list order: %+v", rs)
	}
}

func TestJudgeCheckFilesReadBeforeCommandChecks(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "a.md", "original")
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	checks := []Check{
		{Criterion: "c", Type: CheckCommand, Run: "echo changed > a.md"},
		judgeCheck("q", "a.md"),
	}
	EvaluateChecks(checks, CheckInput{Dir: dir, Judge: f.judge})
	if len(f.queries) != 1 || f.queries[0].Files[0].Content != "original" {
		t.Errorf("judge saw %+v, want the file as the agent left it", f.queries)
	}
}

func TestJudgeCheckInvalidCheckFailsWithoutCalling(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, Check{Criterion: "c", Type: CheckJudge, Question: "  "}, CheckInput{Judge: f.judge})
	wantFail(t, r, "invalid check")
	r = evalOne(t, judgeCheck("q", "../x"), CheckInput{Judge: f.judge})
	wantFail(t, r, "invalid check")
	if len(f.queries) != 0 {
		t.Error("judge invoked for an invalid check")
	}
}

func TestJudgeCheckEvaluatorStaysIndependent(t *testing.T) {
	// A non-judge check ignores Input/Judge entirely.
	f := &fakeJudge{}
	r := evalOne(t, Check{Criterion: "c", Type: CheckOutputContains, Pattern: "x"}, CheckInput{Output: "x", Input: "i", Judge: f.judge})
	wantPass(t, r)
	if len(f.queries) != 0 {
		t.Error("non-judge check invoked the judge")
	}
}

func TestParseYesNo(t *testing.T) {
	wrap := func(body string) string { return "noise\n===JSON_START===\n" + body + "\n===JSON_END===\ntrailer" }
	tests := []struct {
		name      string
		raw       string
		wantYes   bool
		wantReas  string
		wantError string // "" = ok
	}{
		{"yes", wrap(`{"reasoning":"because","answer":"yes"}`), true, "because", ""},
		{"no", wrap(`{"reasoning":"nope","answer":"no"}`), false, "nope", ""},
		{"YES upper", wrap(`{"reasoning":"r","answer":"YES"}`), true, "r", ""},
		{"No mixed", wrap(`{"reasoning":"r","answer":"No"}`), false, "r", ""},
		{"spaces", wrap(`{"reasoning":"r","answer":"  yEs \t"}`), true, "r", ""},
		{"answer before reasoning", wrap(`{"answer":"yes","reasoning":"r"}`), true, "r", ""},
		{"empty reasoning", wrap(`{"reasoning":"","answer":"yes"}`), true, "", ""},
		{"no reasoning key", wrap(`{"answer":"no"}`), false, "", ""},
		{"last START wins", "===JSON_START=== quoted ===JSON_END=== text\n===JSON_START===\n{\"reasoning\":\"real\",\"answer\":\"no\"}\n===JSON_END===", false, "real", ""},
		{"last START wins over earlier valid", "===JSON_START==={\"answer\":\"yes\"}===JSON_END===\n===JSON_START==={\"answer\":\"no\",\"reasoning\":\"r\"}===JSON_END===", false, "r", ""},
		{"ANSI noise", wrap("\x1b[32m{\"reasoning\":\"r\",\"answer\":\"yes\"}\x1b[0m"), true, "r", ""},
		{"maybe", wrap(`{"reasoning":"r","answer":"maybe"}`), false, "", "answer"},
		{"yes please", wrap(`{"reasoning":"r","answer":"yes please"}`), false, "", "answer"},
		{"empty answer", wrap(`{"reasoning":"r","answer":""}`), false, "", "answer"},
		{"missing answer", wrap(`{"reasoning":"r"}`), false, "", "answer"},
		{"answer not a string", wrap(`{"reasoning":"r","answer":true}`), false, "", ""},
		{"invalid JSON", wrap(`{not json`), false, "", "JSON"},
		{"no delimiters", `{"reasoning":"r","answer":"yes"}`, false, "", "delimiters"},
		{"no START", "{\"answer\":\"yes\"}\n===JSON_END===", false, "", "delimiters"},
		{"no END", "===JSON_START==={\"answer\":\"yes\"}", false, "", "delimiters"},
		{"END only before last START", "===JSON_END===\n===JSON_START==={\"answer\":\"yes\"}", false, "", "delimiters"},
		{"empty", "", false, "", "delimiters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseYesNo(tt.raw)
			if tt.wantError == "" && tt.name != "answer not a string" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if v.Yes != tt.wantYes || v.Reasoning != tt.wantReas {
					t.Errorf("got %+v, want yes=%v reasoning=%q", v, tt.wantYes, tt.wantReas)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error, got %+v", v)
			}
			if !errors.Is(err, errJudgeParse) {
				t.Errorf("error does not wrap errJudgeParse: %v", err)
			}
			if tt.wantError != "" && !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error %q does not contain %q", err, tt.wantError)
			}
		})
	}
}
