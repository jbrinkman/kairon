package cmd

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jbrinkman/kairon/internal/templates"
)

func withTemplates(t *testing.T, fsys fstest.MapFS) {
	t.Helper()
	prev := templates.Templates
	templates.Templates = fsys
	t.Cleanup(func() { templates.Templates = prev })
}

func ignoreLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// kairon init keeps the kairon bin directory (.kairon/bin/, which holds a
// symlink to the local binary) out of the project's commits.
func TestInitIgnoresKaironBinDir(t *testing.T) {
	t.Run("new project", func(t *testing.T) {
		t.Chdir(t.TempDir())
		withTemplates(t, fstest.MapFS{"templates/kairon/config.yaml": {Data: []byte("x: 1\n")}})
		if err := initCmd.RunE(initCmd, nil); err != nil {
			t.Fatal(err)
		}
		if got := ignoreLines(t); !strings.Contains(strings.Join(got, "\n"), ".kairon/bin/") {
			t.Fatalf(".gitignore = %q, want an entry for .kairon/bin/", got)
		}
	})

	t.Run("existing gitignore is kept and not duplicated", func(t *testing.T) {
		t.Chdir(t.TempDir())
		withTemplates(t, fstest.MapFS{"templates/kairon/config.yaml": {Data: []byte("x: 1\n")}})
		if err := os.WriteFile(".gitignore", []byte("node_modules/"), 0o644); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := initCmd.RunE(initCmd, nil); err != nil {
				t.Fatal(err)
			}
		}
		got := ignoreLines(t)
		want := []string{"node_modules/", ".kairon/bin/"}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf(".gitignore lines = %q, want %q", got, want)
		}
	})

	// Git keeps leading spaces as part of a pattern but drops trailing ones,
	// so only an exact match (ignoring trailing whitespace) counts as present.
	t.Run("indented entry does not count, trailing whitespace does", func(t *testing.T) {
		for _, tc := range []struct {
			existing, want string
		}{
			{"  .kairon/bin/\n", "  .kairon/bin/\n.kairon/bin/\n"},
			{".kairon/bin/ \r\n", ".kairon/bin/ \r\n"},
		} {
			t.Chdir(t.TempDir())
			withTemplates(t, fstest.MapFS{"templates/kairon/config.yaml": {Data: []byte("x: 1\n")}})
			if err := os.WriteFile(".gitignore", []byte(tc.existing), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := initCmd.RunE(initCmd, nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(".gitignore")
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.want {
				t.Errorf("existing %q: .gitignore = %q, want %q", tc.existing, data, tc.want)
			}
		}
	})
}
