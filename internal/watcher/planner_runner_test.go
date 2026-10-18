package watcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github"
)

func TestKiroPlannerRunner_ImplementsPlannerRunner(t *testing.T) {
	var _ PlannerRunner = (*kiroPlannerRunner)(nil)
}

func TestKiroPlannerRunner_Command(t *testing.T) {
	tests := []struct {
		name       string
		provenance Provenance
		word       string
		reason     string
	}{
		{"none", ProvenanceNone, "none", "no planner marker"},
		{"mismatch", ProvenanceMismatch, "mismatch", "edited after generation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newKiroPlannerRunner("owner/repo")
			cmd := r.command(github.Issue{Number: 368}, tt.provenance)

			if cmd.Process != nil {
				t.Fatal("command must build the process, not start it")
			}
			wantPrefix := []string{"kiro-cli", "chat", "--agent", "planner", "--no-interactive", "--trust-all-tools"}
			if len(cmd.Args) != len(wantPrefix)+1 {
				t.Fatalf("args = %q, want %d fixed args plus one prompt", cmd.Args, len(wantPrefix))
			}
			if !reflect.DeepEqual(cmd.Args[:len(wantPrefix)], wantPrefix) {
				t.Errorf("args prefix = %q, want %q", cmd.Args[:len(wantPrefix)], wantPrefix)
			}

			prompt := cmd.Args[len(cmd.Args)-1]
			for _, want := range []string{"#368", "owner/repo", tt.word, tt.reason, "canonical"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt %q does not contain %q", prompt, want)
				}
			}
		})
	}
}

func TestKiroPlannerRunner_CommandPromptNamesOnlyItsProvenance(t *testing.T) {
	r := newKiroPlannerRunner("owner/repo")

	none := r.command(github.Issue{Number: 7}, ProvenanceNone)
	if strings.Contains(none.Args[len(none.Args)-1], "edited after generation") {
		t.Error("none prompt must not describe the mismatch reason")
	}
	mismatch := r.command(github.Issue{Number: 7}, ProvenanceMismatch)
	if strings.Contains(mismatch.Args[len(mismatch.Args)-1], "no planner marker") {
		t.Error("mismatch prompt must not describe the none reason")
	}
}

func TestKiroPlannerRunner_CommandEnv(t *testing.T) {
	r := newKiroPlannerRunner("owner/repo")
	cmd := r.command(github.Issue{Number: 368}, ProvenanceNone)

	for _, want := range []string{
		"ISSUE_NUMBER=368",
		"REPO=owner/repo",
		fmt.Sprintf("KAIRON_WATCHER_PID=%d", os.Getpid()),
	} {
		if !containsString(cmd.Env, want) {
			t.Errorf("env does not contain %q", want)
		}
	}
}

func TestNewKiroPlannerRunner_DefaultsRunToCmdRun(t *testing.T) {
	r := newKiroPlannerRunner("owner/repo")
	if r.repo != "owner/repo" {
		t.Errorf("repo = %q, want owner/repo", r.repo)
	}
	if r.run == nil {
		t.Fatal("run must default to a non-nil function")
	}

	// A command that cannot start proves the default delegates to (*exec.Cmd).Run
	// without launching anything real.
	bad := exec.Command(filepath.Join(t.TempDir(), "does-not-exist"))
	if err := r.run(bad); err == nil {
		t.Error("default run on a missing executable returned nil, want the Cmd.Run error")
	}
}

func TestKiroPlannerRunner_Run(t *testing.T) {
	t.Run("creates log, calls run once, returns its error", func(t *testing.T) {
		t.Chdir(t.TempDir())

		wantErr := errors.New("planner exploded")
		calls := 0
		r := newKiroPlannerRunner("owner/repo")
		r.run = func(cmd *exec.Cmd) error {
			calls++
			return wantErr
		}

		err := r.Run(github.Issue{Number: 368}, ProvenanceNone)

		if err != wantErr {
			t.Errorf("Run error = %v, want %v unchanged", err, wantErr)
		}
		if calls != 1 {
			t.Errorf("injected run called %d times, want 1", calls)
		}
		logPath := filepath.Join(".kairon", "logs", "planner-issue-368.log")
		if _, statErr := os.Stat(logPath); statErr != nil {
			t.Errorf("log file %s not created: %v", logPath, statErr)
		}
	})

	t.Run("nil error is returned as nil", func(t *testing.T) {
		t.Chdir(t.TempDir())

		r := newKiroPlannerRunner("owner/repo")
		r.run = func(*exec.Cmd) error { return nil }

		if err := r.Run(github.Issue{Number: 5}, ProvenanceMismatch); err != nil {
			t.Errorf("Run error = %v, want nil", err)
		}
	})

	t.Run("passes the built command with stdout and stderr on the log", func(t *testing.T) {
		t.Chdir(t.TempDir())

		r := newKiroPlannerRunner("owner/repo")
		r.run = func(cmd *exec.Cmd) error {
			if cmd.Args[0] != "kiro-cli" || !containsString(cmd.Args, "planner") {
				t.Errorf("run received unexpected command args %q", cmd.Args)
			}
			if cmd.Stdout == nil || cmd.Stderr == nil {
				t.Fatal("Stdout and Stderr must both be set to the log file")
			}
			fmt.Fprint(cmd.Stdout, "out;")
			fmt.Fprint(cmd.Stderr, "err;")
			return nil
		}

		if err := r.Run(github.Issue{Number: 9}, ProvenanceNone); err != nil {
			t.Fatalf("Run: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(".kairon", "logs", "planner-issue-9.log"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "out;err;" {
			t.Errorf("log = %q, want %q", data, "out;err;")
		}
	})

	t.Run("appends across runs", func(t *testing.T) {
		t.Chdir(t.TempDir())

		r := newKiroPlannerRunner("owner/repo")
		r.run = func(cmd *exec.Cmd) error {
			fmt.Fprint(cmd.Stdout, "run;")
			return nil
		}

		for i := 0; i < 2; i++ {
			if err := r.Run(github.Issue{Number: 11}, ProvenanceNone); err != nil {
				t.Fatalf("Run %d: %v", i, err)
			}
		}

		data, err := os.ReadFile(filepath.Join(".kairon", "logs", "planner-issue-11.log"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "run;run;" {
			t.Errorf("log = %q, want both runs appended", data)
		}
	})

	t.Run("log open failure is returned and run is not called", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// A regular file where the .kairon directory should be makes MkdirAll fail.
		if err := os.WriteFile(".kairon", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		called := false
		r := newKiroPlannerRunner("owner/repo")
		r.run = func(*exec.Cmd) error { called = true; return nil }

		if err := r.Run(github.Issue{Number: 3}, ProvenanceNone); err == nil {
			t.Error("Run returned nil, want a log file error")
		}
		if called {
			t.Error("run must not be called when the log cannot be opened")
		}
	})
}

func TestNew_WiresProductionPlannerRunner(t *testing.T) {
	w := New(&config.Config{Repo: "owner/repo", MaxRetries: 3, PollInterval: time.Minute}, nil, "")

	r, ok := w.planner.(*kiroPlannerRunner)
	if !ok {
		t.Fatalf("planner = %T, want *kiroPlannerRunner", w.planner)
	}
	if r.repo != "owner/repo" {
		t.Errorf("runner repo = %q, want owner/repo", r.repo)
	}
	if w.classifier != nil {
		t.Error("New must leave the classifier nil so behaviour is unchanged")
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
