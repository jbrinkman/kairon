package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	timestampedDirRe = regexp.MustCompile(`^\d{6}-\d{6}-[a-f0-9]+$`)
	hashOnlyRe       = regexp.MustCompile(`^[a-f0-9]+$`)
)

const timestampLayout = "060102-150405"

// generateTimestampPrefix returns UTC timestamp in YYMMDD-HHMMSS format
func generateTimestampPrefix() string {
	return time.Now().UTC().Format(timestampLayout)
}

// makeUniqueResultsDir creates a fresh results directory named `name` under
// `parent` and returns its full path. The run-directory name is derived from a
// one-second-granularity timestamp (see generateTimestampPrefix), so two runs
// that start within the same wall-clock second would otherwise collide on the
// same name; a plain os.MkdirAll would then silently reuse the first run's
// directory and the second run's output would overwrite or vanish. That is the
// race that made the self-test's two-run checks flaky in CI
// ("created 0 result dirs, want 1").
//
// To stay collision-proof while preserving the YYMMDD-HHMMSS[-hash] name
// format that parseDirectoryName and the diff tooling recognise, this creates
// the directory with os.Mkdir (which fails rather than reusing an existing
// directory) and, on a collision, advances the timestamp portion of the name by
// one second and retries. The git-hash suffix (when present) is preserved.
func makeUniqueResultsDir(parent, name string) (string, error) {
	ts, suffix := splitRunDirName(name)
	for attempt := 0; attempt < 86400; attempt++ {
		candidate := ts.Format(timestampLayout) + suffix
		dir := filepath.Join(parent, candidate)
		err := os.Mkdir(dir, 0755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			// The parent may not exist yet (first run of a session): create the
			// whole chain once, then retry the leaf with os.Mkdir so collision
			// detection on the leaf is preserved.
			if os.IsNotExist(err) {
				if mkErr := os.MkdirAll(parent, 0755); mkErr != nil {
					return "", mkErr
				}
				continue
			}
			return "", err
		}
		// Directory already exists: advance one second and try the next name.
		ts = ts.Add(time.Second)
	}
	return "", fmt.Errorf("could not allocate a unique results directory under %s for %q", parent, name)
}

// splitRunDirName splits a run-directory name into its parsed timestamp and the
// remainder (e.g. "-<githash>"). A name whose leading YYMMDD-HHMMSS does not
// parse is treated as having no timestamp: it is returned whole as the suffix
// with a zero time, so the caller still gets deterministic, collision-free
// retries (the zero time simply advances by one second each attempt).
func splitRunDirName(name string) (time.Time, string) {
	// The timestamp is the first two dash-separated fields: YYMMDD-HHMMSS.
	parts := strings.SplitN(name, "-", 3)
	if len(parts) >= 2 {
		stamp := parts[0] + "-" + parts[1]
		if ts, err := time.ParseInLocation(timestampLayout, stamp, time.UTC); err == nil {
			suffix := strings.TrimPrefix(name, stamp)
			return ts, suffix
		}
	}
	return time.Time{}, name
}

// parseDirectoryName extracts commit hash from both old and new formats
func parseDirectoryName(dirName string) string {
	// New format: YYMMDD-HHMMSS-{hash}
	if timestampedDirRe.MatchString(dirName) {
		parts := strings.Split(dirName, "-")
		if len(parts) == 3 {
			return parts[2]
		}
	}

	// Old format: just {hash}
	if hashOnlyRe.MatchString(dirName) {
		return dirName
	}

	return ""
}
