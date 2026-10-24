package boundary

import (
	"strings"
	"testing"
)

func TestDefaultScreen_Accepts(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"plain":            "## Title\n\nDo the thing.",
		"tabs and newline": "a\tb\nc",
		"unicode":          "héllo wörld — 日本語 🎉",
		"markdown":         "- [ ] item\n```go\nfmt.Println(\"x\")\n```",
		"exactly the max":  strings.Repeat("a", MaxBodyBytes),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := DefaultScreen(body); err != nil {
				t.Errorf("DefaultScreen rejected an acceptable body: %v", err)
			}
		})
	}
}

func TestDefaultScreen_Rejects(t *testing.T) {
	cases := map[string]string{
		"invalid utf-8":     "ok \xff\xfe bad",
		"truncated utf-8":   "ok \xe3\x81",
		"NUL":               "a\x00b",
		"SOH":               "a\x01b",
		"escape":            "a\x1b[31mred",
		"DEL":               "a\x7fb",
		"carriage return":   "a\rb",
		"C1 control":        "a\u0085b",
		"one byte over max": strings.Repeat("a", MaxBodyBytes+1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := DefaultScreen(body); err == nil {
				t.Error("DefaultScreen accepted a body it must reject")
			}
		})
	}
}

func TestDefaultScreen_ErrorNamesTheProblemNotTheContent(t *testing.T) {
	err := DefaultScreen("secret-token\x00")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error %q echoes the body", err)
	}
}
