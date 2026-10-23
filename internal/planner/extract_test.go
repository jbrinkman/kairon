package planner

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// miniDoc is a small, valid canonical document used as the base of the
// table tests. It ends with a newline.
const miniDoc = "schema_version: 1\n" +
	"stage: planner\n" +
	"result: canonical\n" +
	"source:\n" +
	"  kind: document\n" +
	"  ref: \"interactive-request\"\n" +
	"generated: \"2026-10-21T12:00:00Z\"\n" +
	"body: |\n" +
	"  # Title\n" +
	"\n" +
	"  ## Problem Statement\n" +
	"\n" +
	"  Something.\n"

func TestExtract(t *testing.T) {
	t.Parallel()

	// A body that itself contains an indented code fence must not confuse
	// the fence detection: only column-0 fence lines count.
	docWithInnerFence := strings.Replace(miniDoc, "  Something.\n",
		"  Run:\n\n  ```bash\n  go test ./...\n  ```\n", 1)

	tests := []struct {
		name    string
		reply   string
		want    string
		wantErr error
	}{
		{name: "bare document", reply: miniDoc, want: miniDoc},
		{name: "bare document without trailing newline", reply: strings.TrimSuffix(miniDoc, "\n"), want: miniDoc},
		{name: "bare document with surrounding blank lines", reply: "\n\n  \n" + miniDoc + "\n\n \n", want: miniDoc},
		{name: "bare document with CRLF line endings", reply: strings.ReplaceAll(miniDoc, "\n", "\r\n"), want: miniDoc},
		{name: "yaml fence", reply: "```yaml\n" + miniDoc + "```\n", want: miniDoc},
		{name: "yml fence", reply: "```yml\n" + miniDoc + "```\n", want: miniDoc},
		{name: "fence with blank lines around it", reply: "\n\n```yaml\n" + miniDoc + "```\n\n\n", want: miniDoc},
		{name: "fence with trailing space on fence lines", reply: "```yaml  \n" + miniDoc + "```  \n", want: miniDoc},
		{name: "fence with blank lines inside it", reply: "```yaml\n\n" + miniDoc + "\n```", want: miniDoc},
		{name: "indented code fence inside the body", reply: docWithInnerFence, want: docWithInnerFence},
		{name: "indented code fence inside a fenced document", reply: "```yaml\n" + docWithInnerFence + "```\n", want: docWithInnerFence},

		{name: "prose before a bare document", reply: "Here is the issue:\n\n" + miniDoc, wantErr: ErrExtraneousText},
		{name: "key-like prose before a bare document", reply: "Note: here it is\n" + miniDoc, wantErr: ErrExtraneousText},
		{name: "prose after a bare document", reply: miniDoc + "\nLet me know if you want changes.\n", wantErr: ErrExtraneousText},
		{name: "key-like prose after a bare document", reply: miniDoc + "Note: that is all\n", wantErr: ErrExtraneousText},
		{name: "prose before a fence", reply: "Here is the issue:\n```yaml\n" + miniDoc + "```\n", wantErr: ErrExtraneousText},
		{name: "prose after a fence", reply: "```yaml\n" + miniDoc + "```\nThat is the draft.\n", wantErr: ErrExtraneousText},
		{name: "second fence", reply: "```yaml\n" + miniDoc + "```\n\n```yaml\n" + miniDoc + "```\n", wantErr: ErrExtraneousText},
		{name: "second fence with another language", reply: "```yaml\n" + miniDoc + "```\n```bash\nls\n```\n", wantErr: ErrExtraneousText},
		{name: "two documents separated by a marker", reply: miniDoc + "---\n" + miniDoc, wantErr: ErrExtraneousText},
		{name: "two documents back to back", reply: miniDoc + miniDoc, wantErr: ErrExtraneousText},
		{name: "two documents in one fence", reply: "```yaml\n" + miniDoc + "---\n" + miniDoc + "```\n", wantErr: ErrExtraneousText},
		{name: "fence with another language", reply: "```json\n" + miniDoc + "```\n", wantErr: ErrExtraneousText},
		{name: "bare fence with no language", reply: "```\n" + miniDoc + "```\n", wantErr: ErrExtraneousText},
		{name: "reply that is plain prose", reply: "I could not decide.\n", wantErr: ErrExtraneousText},
		{name: "fenced content that is prose", reply: "```yaml\nHello there\n```\n", wantErr: ErrExtraneousText},

		{name: "empty reply", reply: "", wantErr: ErrNoDocument},
		{name: "whitespace-only reply", reply: " \n\t\n\n", wantErr: ErrNoDocument},
		{name: "fence not closed", reply: "```yaml\n" + miniDoc, wantErr: ErrNoDocument},
		{name: "empty fence", reply: "```yaml\n```\n", wantErr: ErrNoDocument},
		{name: "fence holding only blank lines", reply: "```yaml\n\n  \n```\n", wantErr: ErrNoDocument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Extract(tc.reply)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Extract error = %v, want errors.Is %v (text %q)", err, tc.wantErr, got)
				}
				if got != "" {
					t.Errorf("Extract returned text %q alongside an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract: unexpected error %v", err)
			}
			if got != tc.want {
				t.Errorf("Extract text mismatch\n got: %q\nwant: %q", got, tc.want)
			}
			if _, err := stageoutput.Parse([]byte(got)); err != nil {
				t.Errorf("extracted text does not parse: %v", err)
			}
		})
	}
}

func TestExtractErrorsAreDistinct(t *testing.T) {
	t.Parallel()
	if errors.Is(ErrNoDocument, ErrExtraneousText) || errors.Is(ErrExtraneousText, ErrNoDocument) {
		t.Fatal("ErrNoDocument and ErrExtraneousText must be distinct")
	}
}

// plannerCase is the slice of an eval case file this test needs. It is decoded
// here rather than through internal/eval, which this package must not import.
type plannerCase struct {
	Stub struct {
		Turns []struct {
			Response string `yaml:"response"`
		} `yaml:"turns"`
	} `yaml:"stub"`
}

// beginsDocument reports whether a stub reply starts a stage-output document,
// bare or inside a yaml fence.
func beginsDocument(reply string) bool {
	s := strings.TrimSpace(reply)
	for _, prefix := range []string{"schema_version:", "```yaml\nschema_version:", "```yml\nschema_version:"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func TestExtractReferenceDocuments(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join(repoRoot(t), ".kairon", "evals", "cases", "planner", "*.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no planner eval cases found")
	}

	found := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var c plannerCase
		if err := yaml.Unmarshal(data, &c); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for i, turn := range c.Stub.Turns {
			if !beginsDocument(turn.Response) {
				continue
			}
			found++
			name := filepath.Base(file)
			t.Run(name+"/turn"+string(rune('1'+i)), func(t *testing.T) {
				text, err := Extract(turn.Response)
				if err != nil {
					t.Fatalf("Extract: %v", err)
				}
				if _, err := stageoutput.Parse([]byte(text)); err != nil {
					t.Fatalf("stageoutput.Parse: %v", err)
				}
				// The same reply wrapped in a fence extracts to the same text.
				fenced, err := Extract("```yaml\n" + text + "```\n")
				if err != nil {
					t.Fatalf("Extract (fenced): %v", err)
				}
				if fenced != text {
					t.Errorf("fenced extraction differs from bare extraction")
				}
			})
		}
	}
	if found < 5 {
		t.Fatalf("found %d reference documents in planner cases, want at least 5", found)
	}
}

func TestPlannerDoesNotImportEval(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		parsed, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range parsed.Imports {
			if path := strings.Trim(imp.Path.Value, `"`); strings.HasSuffix(path, "/internal/eval") || strings.Contains(path, "/internal/eval/") {
				t.Errorf("%s imports %s", f, path)
			}
		}
	}
}
