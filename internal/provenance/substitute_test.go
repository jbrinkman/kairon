package provenance

import (
	"errors"
	"strings"
	"testing"
)

func TestSubstituteDependencies_ReplacesPlaceholders(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		numbers map[string]int
		want    string
	}{
		{
			name:    "placeholder and real number",
			body:    "Dependencies: #{a}, #314",
			numbers: map[string]int{"a": 7},
			want:    "Dependencies: #7, #314",
		},
		{
			name:    "duplicate from placeholder and real number removed",
			body:    "Dependencies: #{a}, #7, #{a}",
			numbers: map[string]int{"a": 7},
			want:    "Dependencies: #7",
		},
		{
			name:    "two keys resolving to the same number are deduplicated",
			body:    "Dependencies: #{a}, #{b}",
			numbers: map[string]int{"a": 9, "b": 9},
			want:    "Dependencies: #9",
		},
		{
			name:    "order of first occurrence is kept",
			body:    "Dependencies: #5, #{b}, #{a}, #5",
			numbers: map[string]int{"a": 1, "b": 2},
			want:    "Dependencies: #5, #2, #1",
		},
		{
			name:    "keyword casing and prefix preserved, spacing normalized",
			body:    "- DEPENDENCIES:    #{a},#{b}",
			numbers: map[string]int{"a": 1, "b": 2},
			want:    "- DEPENDENCIES: #1, #2",
		},
		{
			name:    "keys with digits, underscore and dash",
			body:    "Dependencies: #{0-schema_v2}",
			numbers: map[string]int{"0-schema_v2": 12},
			want:    "Dependencies: #12",
		},
		{
			name:    "only the dependencies line changes",
			body:    "# Title\n\nSome text.\nDependencies: #{a}\n\nMore text.\n",
			numbers: map[string]int{"a": 3},
			want:    "# Title\n\nSome text.\nDependencies: #3\n\nMore text.\n",
		},
		{
			name:    "CRLF line ending preserved",
			body:    "Dependencies: #{a}\r\nrest\r\n",
			numbers: map[string]int{"a": 3},
			want:    "Dependencies: #3\r\nrest\r\n",
		},
		{
			name:    "multiple dependencies lines are each rewritten",
			body:    "Dependencies: #{a}\nBlocked text\nDependencies: #{b}, #{a}",
			numbers: map[string]int{"a": 1, "b": 2},
			want:    "Dependencies: #1\nBlocked text\nDependencies: #2, #1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SubstituteDependencies(c.body, c.numbers)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubstituteDependencies_OnlyDependenciesLine(t *testing.T) {
	cases := []string{
		"Use `puts \"#{x}\"` in the example.",
		"```ruby\nname = \"#{x}\"\nputs \"hi #{name}\"\n```",
		"Prose mentioning #{x} and #{y} but no keyword.",
		"## Dependencies\n- #{x} must merge first",
	}
	for _, body := range cases {
		got, err := SubstituteDependencies(body, map[string]int{"x": 1})
		if err != nil {
			t.Errorf("body %q: unexpected error: %v", body, err)
			continue
		}
		if got != body {
			t.Errorf("body %q changed to %q", body, got)
		}
	}

	// A real dependencies line is rewritten while an interpolation elsewhere
	// in the same body stays byte-for-byte identical.
	body := "puts \"#{x}\"\nDependencies: #{x}\nputs \"#{x}\""
	want := "puts \"#{x}\"\nDependencies: #4\nputs \"#{x}\""
	got, err := SubstituteDependencies(body, map[string]int{"x": 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteDependencies_UnknownPlaceholder(t *testing.T) {
	t.Run("unknown key", func(t *testing.T) {
		_, err := SubstituteDependencies("Dependencies: #{missing}", map[string]int{"a": 1})
		if !errors.Is(err, ErrUnknownPlaceholder) {
			t.Fatalf("err = %v, want ErrUnknownPlaceholder", err)
		}
		if err != nil && !strings.Contains(err.Error(), "missing") {
			t.Errorf("error %q should name the key", err)
		}
	})
	t.Run("forward reference", func(t *testing.T) {
		// "b" is a later item in the chain, so it is not in the map yet.
		_, err := SubstituteDependencies("Dependencies: #{a}, #{b}", map[string]int{"a": 1})
		if !errors.Is(err, ErrUnknownPlaceholder) {
			t.Fatalf("err = %v, want ErrUnknownPlaceholder", err)
		}
	})
	t.Run("nil map", func(t *testing.T) {
		_, err := SubstituteDependencies("Dependencies: #{a}", nil)
		if !errors.Is(err, ErrUnknownPlaceholder) {
			t.Fatalf("err = %v, want ErrUnknownPlaceholder", err)
		}
	})
}

func TestSubstituteDependencies_BadLine(t *testing.T) {
	cases := []string{
		"Dependencies: #{a}, none",
		"Dependencies: none, #{a}",
		"Dependencies: #{a} and #5",
		"Dependencies: #{a},",
		"Dependencies: #{a}, , #5",
		"Dependencies: #{a}, 314",
		"Dependencies: #{a}, #abc",
		"Dependencies: #{a}, #99999999999999999999999",
	}
	for _, body := range cases {
		_, err := SubstituteDependencies(body, map[string]int{"a": 1})
		if !errors.Is(err, ErrBadDependencyLine) {
			t.Errorf("body %q: err = %v, want ErrBadDependencyLine", body, err)
		}
	}
}

func TestSubstituteDependencies_NoPlaceholderUnchanged(t *testing.T) {
	cases := []string{
		"",
		"No dependencies mentioned here.",
		"Dependencies: #314",
		"Dependencies: #314,#315",
		"Dependencies:    #315 ,  #315",
		"Dependencies: none",
		"dependencies:",
		"Dependencies: #{bad key}",
		"## Dependencies\n- #314 must merge first\n",
		"line one\r\nDependencies: none \r\n\r\ntrailing\n",
	}
	for _, body := range cases {
		got, err := SubstituteDependencies(body, map[string]int{"a": 1})
		if err != nil {
			t.Errorf("body %q: unexpected error: %v", body, err)
			continue
		}
		if got != body {
			t.Errorf("body %q changed to %q", body, got)
		}
	}
}

func TestSubstituteDependencies_SingleLineOutput(t *testing.T) {
	body := "## Dependencies\nDependencies: #{a}, #{b}, #3\n"
	got, err := SubstituteDependencies(body, map[string]int{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "## Dependencies\nDependencies: #1, #2, #3\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-") || strings.HasPrefix(strings.TrimSpace(line), "*") {
			t.Errorf("output contains a bullet line %q", line)
		}
	}
	if strings.Contains(got, "#{") {
		t.Errorf("output still contains a placeholder: %q", got)
	}
}
