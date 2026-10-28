package checks

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
			err := Validate("case-file.yaml", checks, ValidateOptions{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error does not wrap ErrInvalid: %v", err)
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

func TestJudgeCheckLabel(t *testing.T) {
	docWithTurn := docCheck()
	docWithTurn.Turn = turnPtr(1)

	cases := []struct {
		name   string
		index  int
		check  Check
		want   string                         // exact label; ignored when verify is set
		verify func(t *testing.T, got string) // custom assertion for labels that are not compared exactly
	}{
		{
			name:  "judge-question",
			index: 2,
			check: judgeCheck("Does the body state a criterion?", "a.md"),
			want:  `#2 judge question="Does the body state a criterion?"`,
		},
		{
			name:  "judge-long-question-shortened",
			index: 1,
			check: judgeCheck(strings.Repeat("q", 200)),
			verify: func(t *testing.T, got string) {
				if len(got) > 120 || !strings.HasSuffix(got, `…"`) {
					t.Errorf("long question not shortened: %q", got)
				}
			},
		},
		{
			name:  "document-result-label",
			index: 2,
			check: docCheck(),
			want:  "#2 output_document_valid",
		},
		{
			name:  "document-label-with-turn",
			index: 1,
			check: docWithTurn,
			want:  "#1 output_document_valid turn=1",
		},
		{
			name:  "fact-not-asked",
			index: 2,
			check: factCheck(CheckFactNotAsked, "f1"),
			want:  "#2 fact_not_asked fact=f1",
		},
		{
			name:  "fact-asked",
			index: 1,
			check: factCheck(CheckFactAsked, "f1"),
			want:  "#1 fact_asked fact=f1",
		},
		{
			// Existing labels are unchanged by the fact check types.
			name:  "file-exists-unchanged",
			index: 3,
			check: Check{Type: CheckFileExists, Path: "a.txt"},
			want:  "#3 file_exists path=a.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkLabel(tc.index, tc.check)
			if tc.verify != nil {
				tc.verify(t, got)
				return
			}
			if got != tc.want {
				t.Errorf("label = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestJudgeCheckYesPasses is the verdict table: a yes verdict passes with the
// reasoning in the detail, and a no verdict fails with the reasoning.
func TestJudgeCheckYesPasses(t *testing.T) {
	tests := []struct {
		name    string
		verdict JudgeVerdict
		verify  func(t *testing.T, r Result)
	}{
		{"verdict-yes", JudgeVerdict{Yes: true, Reasoning: "clearly stated"}, func(t *testing.T, r Result) {
			wantPass(t, r)
			if !strings.Contains(r.Detail, "judge answered yes: clearly stated") {
				t.Errorf("detail %q lacks yes + reasoning", r.Detail)
			}
			if r.Type != CheckJudge || !strings.Contains(r.Label, "judge") {
				t.Errorf("result type/label wrong: %+v", r)
			}
		}},
		{"verdict-no", JudgeVerdict{Yes: false, Reasoning: "criterion missing"}, func(t *testing.T, r Result) {
			wantFail(t, r, "judge answered no: criterion missing")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeJudge{verdict: tt.verdict}
			r := evalOne(t, judgeCheck("Is it good?"), Input{Judge: f.judge})
			tt.verify(t, r)
		})
	}
}

func TestJudgeCheckEmptyReasoning(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, judgeCheck("q"), Input{Judge: f.judge})
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
	r := evalOne(t, c, Input{Dir: dir, Output: "final output", Input: "case input", Judge: f.judge})
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
			r := evalOne(t, judgeCheck("q", tt.files...), Input{Dir: dir, Judge: f.judge})
			wantFail(t, r, tt.want)
			if len(f.queries) != 0 {
				t.Errorf("judge was invoked %d times for a refused file", len(f.queries))
			}
		})
	}
}

func TestJudgeCheckFilesNeedWorkspace(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, judgeCheck("q", "a.md"), Input{Judge: f.judge})
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
		wantPass(t, evalOne(t, judgeCheck("q", "big.md", "small.md"), Input{Dir: dir, Judge: f.judge}))
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
		wantPass(t, evalOne(t, judgeCheck("q", "f0.md", "f1.md", "f2.md", "f3.md", "f4.md"), Input{Dir: dir, Judge: f.judge}))
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

// TestJudgeCheckManyLargeFilesNoPanic covers the case where several files each
// larger than judgeMaxFileBytes exhaust the total budget. The marker must not
// be charged against the budget and the per-file limit must stay non-negative,
// so the read never hits data[:negativeLimit]. Before the shownBytes fix the
// budget went negative after the fourth file and the fifth read panicked
// (recovered by evaluateOne as a check failure), so capping never showed large
// files. Each file here is 70,000 bytes (> judgeMaxFileBytes = 65536).
func TestJudgeCheckManyLargeFilesNoPanic(t *testing.T) {
	dir := t.TempDir()
	const big = 70000
	names := make([]string, 5)
	for i := range names {
		names[i] = fmt.Sprintf("big%d.md", i)
		writeEvalFile(t, dir, names[i], strings.Repeat("a", big))
	}
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	// Passes only if evalJudge did not fail the check (a panic would be
	// recovered as a failure, not a pass).
	wantPass(t, evalOne(t, judgeCheck("q", names...), Input{Dir: dir, Judge: f.judge}))

	files := f.queries[0].Files
	if len(files) != len(names) {
		t.Fatalf("got %d files, want %d", len(files), len(names))
	}
	shown := 0
	for _, jf := range files {
		if !jf.Truncated {
			t.Errorf("%s should be truncated (file is %d bytes, per-file cap is %d)", jf.Path, big, judgeMaxFileBytes)
		}
		body := jf.Content
		if i := strings.Index(body, "\n[truncated:"); i >= 0 {
			body = body[:i]
		}
		shown += len(body)
	}
	if shown > judgeMaxTotalBytes {
		t.Errorf("showed %d bytes of file content, want <= %d", shown, judgeMaxTotalBytes)
	}
}

func TestJudgeCheckTruncatesOnRuneBoundaryAndSanitizesUTF8(t *testing.T) {
	dir := t.TempDir()
	// "é" is 2 bytes, so the 64 KiB cut falls mid-rune for an odd offset.
	writeEvalFile(t, dir, "u.md", "x"+strings.Repeat("é", judgeMaxFileBytes))
	writeEvalFile(t, dir, "bad.md", "ok\xffend")
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	wantPass(t, evalOne(t, judgeCheck("q", "u.md", "bad.md"), Input{Dir: dir, Judge: f.judge}))
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

// TestJudgeCheckInvalidBytesStayWithinCap guards against ToValidUTF8 expansion
// pushing a truncated file past the cap: each invalid byte is replaced with
// U+FFFD (3 bytes), so a prefix of alternating ASCII and invalid bytes capped
// at the raw-byte limit roughly doubles once sanitized. The shown content
// (excluding the marker) must still be <= judgeMaxFileBytes and valid UTF-8.
func TestJudgeCheckInvalidBytesStayWithinCap(t *testing.T) {
	dir := t.TempDir()
	// 2*judgeMaxFileBytes bytes alternating 'a' and 0xff. Capped raw to 64 KiB
	// that is ~32 KiB of 0xff, each becoming a 3-byte U+FFFD -> ~128 KiB.
	buf := make([]byte, 2*judgeMaxFileBytes)
	for i := range buf {
		if i%2 == 0 {
			buf[i] = 'a'
		} else {
			buf[i] = 0xff
		}
	}
	writeEvalFile(t, dir, "x.md", string(buf))
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	wantPass(t, evalOne(t, judgeCheck("q", "x.md"), Input{Dir: dir, Judge: f.judge}))

	jf := f.queries[0].Files[0]
	if !jf.Truncated {
		t.Fatal("file should be truncated")
	}
	shown := jf.Content
	if i := strings.Index(shown, "\n[truncated:"); i >= 0 {
		shown = shown[:i]
	}
	if !isValidUTF8(shown) {
		t.Errorf("shown content is not valid UTF-8")
	}
	if len(shown) > judgeMaxFileBytes {
		t.Errorf("shown content is %d bytes after U+FFFD expansion, exceeds cap %d", len(shown), judgeMaxFileBytes)
	}
}

// TestJudgeCheckCutInsideMultiByteRuneRemovesWholeRune guards against a cut
// landing in the MIDDLE of a 3- or 4-byte rune: the whole incomplete rune must
// be dropped, not just one byte. Otherwise ToValidUTF8 replaces the dangling
// byte(s) with U+FFFD (altered text) and the shown prefix can exceed the cap.
// For "€" (3 bytes) the 64 KiB prefix ends with 2 of its bytes; for "𝄞" (4
// bytes) it can end with 1-3. In both cases the kept prefix must be valid
// UTF-8, carry no U+FFFD, and be <= judgeMaxFileBytes.
func TestJudgeCheckCutInsideMultiByteRuneRemovesWholeRune(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		// 65534 ASCII + "€" (E2 82 AC): the 65536-byte cut keeps "€"'s first 2 bytes.
		"three-byte": strings.Repeat("a", judgeMaxFileBytes-2) + "€",
		// 65533 ASCII + "𝄞" (F0 9D 84 9E): the cut keeps "𝄞"'s first 3 bytes.
		"four-byte": strings.Repeat("a", judgeMaxFileBytes-3) + "𝄞",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			writeEvalFile(t, dir, name+".md", content+strings.Repeat("b", 100))
			f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
			wantPass(t, evalOne(t, judgeCheck("q", name+".md"), Input{Dir: dir, Judge: f.judge}))
			jf := f.queries[0].Files[0]
			shown := jf.Content
			if i := strings.Index(shown, "\n[truncated:"); i >= 0 {
				shown = shown[:i]
			}
			if !isValidUTF8(shown) {
				t.Errorf("shown prefix is not valid UTF-8")
			}
			if strings.Contains(shown, "\uFFFD") {
				t.Errorf("shown prefix contains U+FFFD: a split rune was not fully removed")
			}
			if len(shown) > judgeMaxFileBytes {
				t.Errorf("shown prefix is %d bytes, exceeds cap %d", len(shown), judgeMaxFileBytes)
			}
			// The incomplete trailing rune is gone, so the prefix is all ASCII 'a'.
			if !strings.HasSuffix(shown, "a") {
				t.Errorf("shown prefix should end on the last complete ASCII byte, got ...%q", shown[max(0, len(shown)-4):])
			}
		})
	}
}

// TestJudgeCheckKeepsCompleteFinalRuneAtCap guards the P2 trailing-trim bug:
// when the capped prefix ends exactly on a COMPLETE multi-byte rune, that rune
// must be kept, not dropped. Here (judgeMaxFileBytes-2) ASCII bytes plus one
// 2-byte "é" fill the 64 KiB prefix exactly, so no incomplete rune is present.
// The old loop stripped the continuation byte and then FullRune([lead]) dropped
// the lead byte too, discarding a valid rune and showing less than the cap.
func TestJudgeCheckKeepsCompleteFinalRuneAtCap(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("a", judgeMaxFileBytes-2) + "é" // exactly judgeMaxFileBytes bytes, then more
	writeEvalFile(t, dir, "r.md", content+strings.Repeat("b", 100))
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "r"}}
	wantPass(t, evalOne(t, judgeCheck("q", "r.md"), Input{Dir: dir, Judge: f.judge}))

	jf := f.queries[0].Files[0]
	if !jf.Truncated {
		t.Fatal("file should be truncated")
	}
	shown := jf.Content
	if i := strings.Index(shown, "\n[truncated:"); i >= 0 {
		shown = shown[:i]
	}
	if len(shown) != judgeMaxFileBytes {
		t.Errorf("shown prefix is %d bytes, want exactly %d (a complete final rune was dropped)", len(shown), judgeMaxFileBytes)
	}
	if !strings.HasSuffix(shown, "é") {
		t.Errorf("shown prefix does not end with the complete final rune 'é': ...%q", shown[max(0, len(shown)-4):])
	}
}

func TestJudgeCheckJudgeErrorFails(t *testing.T) {
	f := &fakeJudge{err: errors.New("backend down")}
	r := evalOne(t, judgeCheck("q"), Input{Judge: f.judge})
	wantFail(t, r, "judge call failed: backend down")
}

func TestJudgeCheckParseErrorFails(t *testing.T) {
	f := &fakeJudge{err: fmt.Errorf("%w: JSON delimiters not found", ErrJudgeParse)}
	r := evalOne(t, judgeCheck("q"), Input{Judge: f.judge})
	wantFail(t, r, "judge parse error: JSON delimiters not found")
	if strings.Contains(r.Detail, "judge call failed") {
		t.Errorf("parse error reported as call failure: %q", r.Detail)
	}
	if strings.Contains(r.Detail, "judge parse error: judge parse error") {
		t.Errorf("parse error prefix doubled: %q", r.Detail)
	}
}

func TestJudgeCheckNilJudgeFailsEvenWithFiles(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "a.md", "a")
	r := evalOne(t, judgeCheck("q", "a.md"), Input{Dir: dir})
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
	rs := Evaluate(checks, Input{Dir: dir, Output: "out", Judge: func(q JudgeQuery) (JudgeVerdict, error) {
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
	Evaluate(checks, Input{Dir: dir, Judge: f.judge})
	if len(f.queries) != 1 || f.queries[0].Files[0].Content != "original" {
		t.Errorf("judge saw %+v, want the file as the agent left it", f.queries)
	}
}

func TestJudgeCheckInvalidCheckFailsWithoutCalling(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true}}
	r := evalOne(t, Check{Criterion: "c", Type: CheckJudge, Question: "  "}, Input{Judge: f.judge})
	wantFail(t, r, "invalid check")
	r = evalOne(t, judgeCheck("q", "../x"), Input{Judge: f.judge})
	wantFail(t, r, "invalid check")
	if len(f.queries) != 0 {
		t.Error("judge invoked for an invalid check")
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
			v, err := ParseYesNo(tt.raw)
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
			if !errors.Is(err, ErrJudgeParse) {
				t.Errorf("error does not wrap ErrJudgeParse: %v", err)
			}
			if tt.wantError != "" && !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error %q does not contain %q", err, tt.wantError)
			}
		})
	}
}

// twoTurnJudgeInput is a hand-built 2-turn Input with distinct markers.
func twoTurnJudgeInput(dir string, f *fakeJudge) Input {
	return Input{
		Dir:       dir,
		Output:    "REPLY-TWO",
		Input:     "rendered input",
		UserTurns: []string{"USER-ONE", "USER-TWO"},
		Turns:     []TurnEvidence{{Output: "REPLY-ONE"}, {Output: "REPLY-TWO"}},
		Judge:     f.judge,
	}
}

func TestJudgeCheckTurnFieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		check   Check
		wantErr string // "" = valid
	}{
		{"turn on judge", Check{Type: CheckJudge, Question: "q", Turn: turnPtr(1)}, ""},
		{"turn 3 on judge", Check{Type: CheckJudge, Question: "q", Turn: turnPtr(3)}, ""},
		{"turn 0 on judge", Check{Type: CheckJudge, Question: "q", Turn: turnPtr(0)}, "turn 0 must be >= 1"},
		{"negative turn on judge", Check{Type: CheckJudge, Question: "q", Turn: turnPtr(-1)}, "must be >= 1"},
		{"turn on file_exists", Check{Type: CheckFileExists, Path: "a", Turn: turnPtr(1)}, "turn is not valid for type file_exists"},
		{"turn on file_contains", Check{Type: CheckFileContains, Path: "a", Pattern: "x", Turn: turnPtr(1)}, "turn is not valid for type file_contains"},
		{"turn on command", Check{Type: CheckCommand, Run: "true", Turn: turnPtr(1)}, "turn is not valid for type command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check.Criterion = "clarity"
			err := Validate("case-file.yaml", []Check{tt.check}, ValidateOptions{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestJudgeCheckMultiTurnDefaultsToLastTurn(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	wantPass(t, evalOne(t, judgeCheck("q"), twoTurnJudgeInput(t.TempDir(), f)))
	if len(f.queries) != 1 {
		t.Fatalf("judge called %d times, want 1", len(f.queries))
	}
	q := f.queries[0]
	if q.Turn != 2 || q.Output != "REPLY-TWO" {
		t.Errorf("Turn = %d, Output = %q; want 2, REPLY-TWO", q.Turn, q.Output)
	}
	want := []JudgeTurn{{User: "USER-ONE", Agent: "REPLY-ONE"}, {User: "USER-TWO", Agent: "REPLY-TWO"}}
	if len(q.Transcript) != len(want) || q.Transcript[0] != want[0] || q.Transcript[1] != want[1] {
		t.Errorf("Transcript = %+v, want %+v", q.Transcript, want)
	}
}

func TestJudgeCheckMultiTurnSelectsEarlierTurn(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	c := judgeCheck("q")
	c.Turn = turnPtr(1)
	wantPass(t, evalOne(t, c, twoTurnJudgeInput(t.TempDir(), f)))
	q := f.queries[0]
	if q.Turn != 1 || q.Output != "REPLY-ONE" {
		t.Errorf("Turn = %d, Output = %q; want 1, REPLY-ONE", q.Turn, q.Output)
	}
	if len(q.Transcript) != 1 || q.Transcript[0] != (JudgeTurn{User: "USER-ONE", Agent: "REPLY-ONE"}) {
		t.Fatalf("Transcript = %+v, want only turn 1", q.Transcript)
	}
	if strings.Contains(fmt.Sprintf("%+v", q.Transcript), "TWO") {
		t.Errorf("turn-1 query leaks turn 2: %+v", q.Transcript)
	}
}

func TestJudgeCheckTurnOutOfRangeFailsBeforeFilesAndJudge(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	c := judgeCheck("q", "missing.md") // would fail with "file does not exist" if read
	c.Turn = turnPtr(9)
	r := evalOne(t, c, twoTurnJudgeInput(t.TempDir(), f))
	wantFail(t, r, "turn 9 is out of range: the case has 2 turn(s)")
	if strings.Contains(r.Detail, "does not exist") {
		t.Errorf("files were read before the turn was checked: %q", r.Detail)
	}
	if len(f.queries) != 0 {
		t.Errorf("judge was invoked %d times for an out-of-range turn", len(f.queries))
	}
}

func TestJudgeCheckClassicInputHasNoTranscript(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	in := Input{Dir: t.TempDir(), Output: "final", Input: "case input", Judge: f.judge}
	wantPass(t, evalOne(t, judgeCheck("q"), in))
	q := f.queries[0]
	if len(q.Transcript) != 0 || q.Turn != 0 || q.Input != "case input" || q.Output != "final" {
		t.Errorf("classic query changed: %+v", q)
	}

	// turn: 1 is accepted and changes nothing.
	f.queries = nil
	c := judgeCheck("q")
	c.Turn = turnPtr(1)
	wantPass(t, evalOne(t, c, in))
	q = f.queries[0]
	if len(q.Transcript) != 0 || q.Input != "case input" || q.Output != "final" {
		t.Errorf("classic query with turn: 1 changed: %+v", q)
	}

	// turn: 2 is out of range.
	f.queries = nil
	c.Turn = turnPtr(2)
	wantFail(t, evalOne(t, c, in), "turn 2 is out of range: the case has 1 turn(s)")
	if len(f.queries) != 0 {
		t.Errorf("judge was invoked for an out-of-range turn")
	}
}

func TestJudgeCheckOneElementTurnsHasNoTranscript(t *testing.T) {
	f := &fakeJudge{verdict: JudgeVerdict{Yes: true, Reasoning: "ok"}}
	in := Input{
		Dir: t.TempDir(), Output: "only reply", Input: "Turn 1 (user): hi",
		UserTurns: []string{"hi"}, Turns: []TurnEvidence{{Output: "only reply"}}, Judge: f.judge,
	}
	wantPass(t, evalOne(t, judgeCheck("q"), in))
	q := f.queries[0]
	if len(q.Transcript) != 0 || q.Input != "Turn 1 (user): hi" || q.Output != "only reply" {
		t.Errorf("one-element turns query changed: %+v", q)
	}
	c := judgeCheck("q")
	c.Turn = turnPtr(2)
	wantFail(t, evalOne(t, c, in), "the case has 1 turn(s)")
}
