package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// judgeMaxFileBytes caps how much of one file a judge check shows.
	judgeMaxFileBytes = 64 << 10
	// judgeMaxTotalBytes caps the file content shown across all files.
	judgeMaxTotalBytes = 256 << 10
)

// errJudgeParse marks a judge answer that could not be read as a yes/no
// verdict. A JudgeFunc returns an error wrapping it for an unparseable answer
// (as opposed to a failed call).
var errJudgeParse = errors.New("judge parse error")

// JudgeFile is one workspace file shown to the judge. Content holds at most
// the capped prefix of the file; when the file was cut, Truncated is set, Size
// is the full size in bytes and Content ends with an explicit marker.
type JudgeFile struct {
	Path      string
	Content   string
	Truncated bool
	Size      int64
}

// JudgeQuery is everything handed to a JudgeFunc for one judge check.
type JudgeQuery struct {
	Criterion string
	Question  string
	Input     string // the case input
	Output    string // the agent's final output
	Files     []JudgeFile
}

// JudgeVerdict is a parsed yes/no answer.
type JudgeVerdict struct {
	Yes       bool
	Reasoning string
}

// JudgeFunc answers a judge check. (It is not called CheckJudge: that name is
// the check-type constant.) A returned error wrapping errJudgeParse is
// reported as a parse error, any other error as a failed call.
type JudgeFunc func(JudgeQuery) (JudgeVerdict, error)

// validateJudgeCheck applies the judge-specific rules on top of the generic
// required/allowed field rules: a non-blank question and local, unique,
// non-.git file entries.
func validateJudgeCheck(c *Check) error {
	if strings.TrimSpace(c.Question) == "" {
		return errors.New("question must not be blank")
	}
	seen := map[string]bool{}
	for _, f := range c.Files {
		if !filepath.IsLocal(f) {
			return fmt.Errorf("files entry %q must be relative and stay inside the workspace (no absolute path, no '..')", f)
		}
		clean := filepath.ToSlash(filepath.Clean(f))
		for _, part := range strings.Split(clean, "/") {
			if part == ".git" {
				return fmt.Errorf("files entry %q must not be inside .git", f)
			}
		}
		if seen[clean] {
			return fmt.Errorf("files entry %q is a duplicate", f)
		}
		seen[clean] = true
	}
	return nil
}

// evalJudge runs one judge check. It fails closed: no judge, an unreadable
// file, a failed call or an unparseable answer all fail the check; there is no
// skipped or silent-pass path. Files are loaded (and refused) before the judge
// is called, so a missing artifact costs no model call.
func evalJudge(c Check, in CheckInput) (bool, string) {
	if in.Judge == nil {
		return false, "no judge configured"
	}
	files, detail := loadJudgeFiles(in.Dir, c.Files)
	if detail != "" {
		return false, detail
	}
	verdict, err := in.Judge(JudgeQuery{
		Criterion: c.Criterion,
		Question:  c.Question,
		Input:     in.Input,
		Output:    in.Output,
		Files:     files,
	})
	if err != nil {
		if errors.Is(err, errJudgeParse) {
			return false, "judge parse error: " + strings.TrimPrefix(err.Error(), errJudgeParse.Error()+": ")
		}
		return false, "judge call failed: " + err.Error()
	}
	reasoning := strings.TrimSpace(verdict.Reasoning)
	if reasoning == "" {
		reasoning = "(no reasoning given)"
	}
	answer := "no"
	if verdict.Yes {
		answer = "yes"
	}
	return verdict.Yes, fmt.Sprintf("judge answered %s: %s", answer, reasoning)
}

// loadJudgeFiles reads the listed workspace files in order, honouring the
// per-file and total caps. A non-empty detail means a file was refused.
func loadJudgeFiles(dir string, paths []string) ([]JudgeFile, string) {
	if len(paths) == 0 {
		return nil, ""
	}
	if err := checkDir(dir); err != nil {
		return nil, err.Error()
	}
	files := make([]JudgeFile, 0, len(paths))
	budget := judgeMaxTotalBytes
	for _, p := range paths {
		limit := min(judgeMaxFileBytes, budget)
		f, detail := readJudgeFile(dir, p, limit)
		if detail != "" {
			return nil, detail
		}
		budget -= shownBytes(f)
		files = append(files, f)
	}
	return files, ""
}

// shownBytes is the length of the file content before any truncation marker.
func shownBytes(f JudgeFile) int {
	if !f.Truncated {
		return len(f.Content)
	}
	return len(f.Content) - len(truncationMarker(f, 0))
}

// truncationMarker is appended to a cut file. shown is only used to size it.
func truncationMarker(f JudgeFile, shown int) string {
	return fmt.Sprintf("\n[truncated: showing first %d of %d bytes]", shown, f.Size)
}

// readJudgeFile reads at most limit bytes of the regular, non-symlink file rel
// below dir. A non-empty detail means the read was refused.
func readJudgeFile(dir, rel string, limit int) (JudgeFile, string) {
	abs, info, err := resolveInDir(dir, rel)
	switch {
	case err != nil:
		return JudgeFile{}, fmt.Sprintf("%s: %s", rel, err)
	case info == nil:
		return JudgeFile{}, "file does not exist: " + rel
	case info.Mode()&os.ModeSymlink != 0:
		return JudgeFile{}, rel + ": path is a symlink; refusing to follow it"
	case !info.Mode().IsRegular():
		return JudgeFile{}, rel + ": path is not a regular file"
	}
	f, err := os.Open(abs)
	if err != nil {
		return JudgeFile{}, fmt.Sprintf("%s: cannot open file: %v", rel, err)
	}
	defer f.Close()
	// The file must still be the one we inspected (not swapped for a link).
	if fi, err := f.Stat(); err != nil || !os.SameFile(info, fi) {
		return JudgeFile{}, rel + ": file changed while being read; refusing"
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return JudgeFile{}, fmt.Sprintf("%s: cannot read file: %v", rel, err)
	}
	jf := JudgeFile{Path: rel, Size: info.Size()}
	if len(data) > limit || info.Size() > int64(limit) {
		jf.Truncated = true
		if len(data) > limit {
			data = data[:limit]
		}
		// Do not cut a multi-byte rune in half.
		for len(data) > 0 && !utf8.RuneStart(data[len(data)-1]) {
			data = data[:len(data)-1]
		}
		if len(data) > 0 && !utf8.FullRune(data[len(data)-1:]) {
			data = data[:len(data)-1]
		}
	}
	jf.Content = strings.ToValidUTF8(string(data), "\uFFFD")
	if jf.Truncated {
		jf.Content += truncationMarker(jf, len(jf.Content))
	}
	return jf, ""
}

const (
	judgeStart = "===JSON_START==="
	judgeEnd   = "===JSON_END==="
)

// parseYesNo reads a judge reply of the form
// ===JSON_START==={"reasoning": "...", "answer": "yes"|"no"}===JSON_END===.
// The last START delimiter and the first END after it are used, ANSI escapes
// are stripped, and the answer must be exactly yes or no (any case, surrounding
// whitespace ignored). Every error wraps errJudgeParse.
func parseYesNo(raw string) (JudgeVerdict, error) {
	start := strings.LastIndex(raw, judgeStart)
	if start < 0 {
		return JudgeVerdict{}, fmt.Errorf("%w: JSON delimiters not found in output", errJudgeParse)
	}
	body := raw[start+len(judgeStart):]
	end := strings.Index(body, judgeEnd)
	if end < 0 {
		return JudgeVerdict{}, fmt.Errorf("%w: JSON delimiters not found in output", errJudgeParse)
	}
	body = stripANSISequences(strings.TrimSpace(body[:end]))

	var reply struct {
		Reasoning string  `json:"reasoning"`
		Answer    *string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &reply); err != nil {
		return JudgeVerdict{}, fmt.Errorf("%w: invalid JSON: %v", errJudgeParse, err)
	}
	if reply.Answer == nil {
		return JudgeVerdict{}, fmt.Errorf("%w: answer is missing", errJudgeParse)
	}
	switch strings.ToLower(strings.TrimSpace(*reply.Answer)) {
	case "yes":
		return JudgeVerdict{Yes: true, Reasoning: reply.Reasoning}, nil
	case "no":
		return JudgeVerdict{Yes: false, Reasoning: reply.Reasoning}, nil
	}
	return JudgeVerdict{}, fmt.Errorf("%w: answer must be yes or no, got %q", errJudgeParse, shorten(*reply.Answer, 40))
}
