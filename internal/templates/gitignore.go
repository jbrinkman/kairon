package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// IgnoredPaths are the Kairon runtime paths a project must never commit.
// .kairon/bin/ holds a symlink to the local kairon binary (see
// internal/kaironbin), which is machine-specific.
var IgnoredPaths = []string{".kairon/bin/"}

// EnsureIgnored appends each entry of IgnoredPaths that is missing from
// destDir/.gitignore, creating the file if needed. Existing lines are kept as
// they are, so running it again changes nothing.
func EnsureIgnored(destDir string) error {
	path := filepath.Join(destDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	for i := range lines {
		// Git drops unescaped trailing spaces but keeps leading ones, so an
		// indented line is a different pattern and must not count as present.
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	var missing []string
	for _, entry := range IgnoredPaths {
		if !slices.Contains(lines, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(missing, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("Updated: %s\n", path)
	return nil
}
