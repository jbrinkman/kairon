package inference

import (
	"reflect"
	"strings"
	"testing"
)

func TestNew_KnownBackends(t *testing.T) {
	for _, name := range []string{"kiro-cli", "stub"} {
		b, err := New(name)
		if err != nil {
			t.Fatalf("New(%q) error: %v", name, err)
		}
		if b.Name() != name {
			t.Errorf("New(%q).Name() = %q", name, b.Name())
		}
	}
}

func TestNew_UnknownBackend(t *testing.T) {
	b, err := New("nope")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	want := `unknown backend "nope"; valid backends: kiro-cli, stub`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestNames_Sorted(t *testing.T) {
	got := Names()
	want := []string{"kiro-cli", "stub"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestEstimateUsage(t *testing.T) {
	u := EstimateUsage(strings.Repeat("a", 40), strings.Repeat("b", 10))
	if u.InputTokens != 10 || u.OutputTokens != 2 {
		t.Errorf("got %+v, want 10 in / 2 out", u)
	}
	if u.Source != UsageEstimated {
		t.Errorf("Source = %q, want %q", u.Source, UsageEstimated)
	}
	if z := EstimateUsage("", ""); z.InputTokens != 0 || z.OutputTokens != 0 {
		t.Errorf("empty estimate = %+v", z)
	}
}

func TestTimeoutOrDefault(t *testing.T) {
	if got := timeoutOrDefault(0); got != DefaultTimeout {
		t.Errorf("zero -> %v", got)
	}
	if got := timeoutOrDefault(-1); got != DefaultTimeout {
		t.Errorf("negative -> %v", got)
	}
	if got := timeoutOrDefault(5); got != 5 {
		t.Errorf("5 -> %v", got)
	}
}
