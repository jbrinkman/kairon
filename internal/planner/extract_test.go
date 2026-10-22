package planner

import (
	"strings"
	"testing"
)

func TestBoundaryExtract(t *testing.T) {
	doc := "schema_version: 1\nstage: planner\nresult: canonical\nbody: |\n  # Title\n\n  text\n"
	cases := []struct {
		name    string
		reply   string
		want    string // expected extracted document, "" when an error is expected
		wantErr string
	}{
		{name: "bare", reply: doc, want: doc},
		{name: "bare with surrounding blank lines", reply: "\n\n" + doc + "\n\n", want: doc},
		{name: "yaml fence", reply: "```yaml\n" + doc + "```", want: doc},
		{name: "yaml fence with surrounding whitespace", reply: "  \n```yaml\n" + doc + "```\n\n", want: doc},
		{name: "crlf", reply: strings.ReplaceAll(doc, "\n", "\r\n"), want: doc},
		{name: "empty", reply: "  \n ", wantErr: "empty"},
		{name: "prose before bare", reply: "Here is the plan:\n" + doc, wantErr: "outside the document"},
		{name: "prose after bare", reply: doc + "\nHope this helps!\n", wantErr: "outside the document"},
		{name: "prose before fence", reply: "Here you go:\n```yaml\n" + doc + "```", wantErr: "outside the document"},
		{name: "prose after fence", reply: "```yaml\n" + doc + "```\nLet me know.", wantErr: "fence"},
		{name: "unclosed fence", reply: "```yaml\n" + doc, wantErr: "fence"},
		{name: "wrong fence language", reply: "```json\n" + doc + "```", wantErr: "fence"},
		{name: "two bare documents", reply: doc + "---\n" + doc, wantErr: "separator"},
		{name: "two fenced documents", reply: "```yaml\n" + doc + "```\n\n```yaml\n" + doc + "```", wantErr: "fence"},
		{name: "markdown heading before document", reply: "# Plan\n" + doc, wantErr: "outside the document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Extract(tc.reply)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Extract() = %q, want an error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract() error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Extract() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestBoundaryExtractKeepsBodyBlockWithFenceInside(t *testing.T) {
	// A fenced code block inside the indented body is body content, not a
	// second document.
	doc := "schema_version: 1\nbody: |\n  ```go\n  fmt.Println()\n  ```\n"
	got, err := Extract("```yaml\n" + doc + "```")
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if got != doc {
		t.Errorf("Extract() = %q, want %q", got, doc)
	}
}

func TestBoundarySplitTitle(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantTitle string
		wantRest  string
		wantOK    bool
	}{
		{name: "h1 then blank line", body: "# Add retry\n\n## Problem\ntext\n", wantTitle: "Add retry", wantRest: "## Problem\ntext\n", wantOK: true},
		{name: "h1 then content", body: "# Add retry\n## Problem\ntext", wantTitle: "Add retry", wantRest: "## Problem\ntext", wantOK: true},
		{name: "leading blank lines", body: "\n\n# Add retry\n\nbody", wantTitle: "Add retry", wantRest: "body", wantOK: true},
		{name: "only one blank line removed", body: "# T\n\n\nbody", wantTitle: "T", wantRest: "\nbody", wantOK: true},
		{name: "closing hashes", body: "# Title ##\nbody", wantTitle: "Title", wantRest: "body", wantOK: true},
		{name: "hash inside title kept", body: "# C# tips\nbody", wantTitle: "C# tips", wantRest: "body", wantOK: true},
		{name: "crlf", body: "# Title\r\n\r\nbody\r\n", wantTitle: "Title", wantRest: "body\r\n", wantOK: true},
		{name: "no h1", body: "## Problem\ntext", wantOK: false},
		{name: "h2 is not a title", body: "## Problem\n# Late\n", wantOK: false},
		{name: "text before h1", body: "intro\n# Title\n", wantOK: false},
		{name: "empty title", body: "# \nbody", wantOK: false},
		{name: "empty body", body: "", wantOK: false},
		{name: "hash without space", body: "#Title\nbody", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, rest, ok := SplitTitle(tc.body)
			if ok != tc.wantOK {
				t.Fatalf("SplitTitle() ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", rest, tc.wantRest)
			}
		})
	}
}

func TestBoundaryAutomatedPromptStartsWithIntakeLine(t *testing.T) {
	p := AutomatedPrompt("owner/repo", 42, "Faster export", "Make it fast.")
	if !strings.HasPrefix(p, AutomatedIntakeLine+"\n") {
		t.Errorf("prompt does not start with the intake line:\n%s", p)
	}
	containsAll(t, p, "owner/repo", "42", "Faster export", "Make it fast.")
	if strings.Count(p, AutomatedIntakeLine) != 1 {
		t.Errorf("intake line should appear once")
	}
	if !strings.HasPrefix(AutomatedIntakeLine, "AUTOMATED INTAKE: non-interactive, no human is available to answer questions.") {
		t.Errorf("AutomatedIntakeLine changed: %q", AutomatedIntakeLine)
	}
}
