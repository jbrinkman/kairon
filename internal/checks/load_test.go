package checks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir holds the checks files shared with the `kairon check` tests.
const fixtureDir = "../eval/testdata/check-cli"

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFile_Fixtures(t *testing.T) {
	opts := ValidateOptions{
		HiddenDir:         filepath.Join(fixtureDir, "hidden"),
		OptionalCriterion: true,
	}
	for _, tc := range []struct {
		file string
		n    int
	}{
		{"pass.yaml", 4},
		{"fail.yaml", 2},
		{"judge.yaml", 1},
		{"evidence.yaml", 3},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(fixtureDir, tc.file)
			list, err := LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if len(list) != tc.n {
				t.Fatalf("got %d checks, want %d", len(list), tc.n)
			}
			if err := Validate(path, list, opts); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestLoadFile_InvalidFixtureLoadsButFailsValidate(t *testing.T) {
	path := filepath.Join(fixtureDir, "invalid.yaml")
	list, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile must decode invalid.yaml: %v", err)
	}
	err = Validate(path, list, ValidateOptions{OptionalCriterion: true})
	if err == nil {
		t.Fatal("Validate accepted invalid.yaml")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("error does not wrap ErrInvalid: %v", err)
	}
	if !strings.Contains(err.Error(), "check #2 (file_contains)") {
		t.Errorf("error %q does not name check #2 (file_contains)", err)
	}
}

func TestLoadFile_EvidenceNeedsHiddenDirForInject(t *testing.T) {
	path := filepath.Join(fixtureDir, "evidence.yaml")
	list, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(path, list, ValidateOptions{OptionalCriterion: true}); err == nil {
		t.Fatal("inject without a hidden dir must be invalid")
	}
}

func TestLoadFile_Rejections(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.yaml")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxCheckFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"unknown key", writeTemp(t, "unknown.yaml", "- type: file_exists\n  path: a\n  patern: x\n"), "patern"},
		{"empty list", writeTemp(t, "empty.yaml", "[]\n"), "no checks"},
		{"empty file", writeTemp(t, "blank.yaml", ""), "no checks"},
		{"not a list", writeTemp(t, "map.yaml", "type: file_exists\npath: a\n"), ""},
		{"directory", dir, "regular file"},
		{"missing", filepath.Join(dir, "nope.yaml"), ""},
		{"too large", big, "10 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list, err := LoadFile(tc.path)
			if err == nil {
				t.Fatalf("LoadFile succeeded with %d checks", len(list))
			}
			if !strings.Contains(err.Error(), tc.path) {
				t.Errorf("error %q does not name the file %q", err, tc.path)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestLoadFile_RejectsTrailingDocuments(t *testing.T) {
	// A valid first document followed by a second (even one that would fail
	// validation) must be an error: the contract is that the whole file is
	// validated, so a silently-ignored trailing document can never happen.
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "second list",
			body: "- type: file_exists\n  path: a\n---\n- type: file_exists\n  path: b\n",
			want: "multiple YAML documents",
		},
		{
			name: "invalid second document",
			body: "- type: file_exists\n  path: a\n---\nnot: a list\n",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, "multi.yaml", tc.body)
			list, err := LoadFile(path)
			if err == nil {
				t.Fatalf("LoadFile accepted a multi-document file (%d checks)", len(list))
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestLoadFile_FollowsSymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := writeTemp(t, "real.yaml", "- type: file_exists\n  path: a\n")
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	// A symlink to a regular file is read as that file; it must
	// load rather than crash.
	if _, err := LoadFile(link); err != nil {
		t.Fatalf("LoadFile(symlink to regular file): %v", err)
	}
}

func TestValidate_OptionalCriterion(t *testing.T) {
	path := writeTemp(t, "nocrit.yaml", "- type: file_exists\n  path: a\n")
	list, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(path, list, ValidateOptions{OptionalCriterion: true}); err != nil {
		t.Errorf("OptionalCriterion must accept a check without criterion: %v", err)
	}
	err = Validate(path, list, ValidateOptions{})
	if err == nil || !strings.Contains(err.Error(), "criterion is required") {
		t.Errorf("without OptionalCriterion got %v, want 'criterion is required'", err)
	}
	if err != nil && !errors.Is(err, ErrInvalid) {
		t.Errorf("error does not wrap ErrInvalid: %v", err)
	}
}

func TestFixtures_NoAbsolutePaths(t *testing.T) {
	err := filepath.WalkDir(fixtureDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, bad := range []string{"/Users/", "/home/", "/tmp/", "C:\\"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s contains absolute path marker %q", p, bad)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixtures_Content(t *testing.T) {
	for file, want := range map[string]string{
		"dir/hello.txt":       "hello world\n",
		"dir/sub/data.txt":    "alpha=1\n",
		"output.txt":          "all work done",
		"gh.log":              "gh issue edit 1 --add-label x",
		"hidden/injected.txt": "injected",
	} {
		b, err := os.ReadFile(filepath.Join(fixtureDir, file))
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		if strings.TrimSpace(string(b)) != strings.TrimSpace(want) {
			t.Errorf("%s = %q, want %q", file, b, want)
		}
	}
}
