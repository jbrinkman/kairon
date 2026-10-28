package version

import (
	"testing"

	"golang.org/x/mod/semver"
)

func TestVersionParsedFromJSON(t *testing.T) {
	if Version == "dev" {
		t.Fatal("Version should be parsed from embedded JSON, got fallback \"dev\"")
	}
	if !semver.IsValid("v" + Version) {
		t.Fatalf("Version %q is not valid semver", Version)
	}
}
