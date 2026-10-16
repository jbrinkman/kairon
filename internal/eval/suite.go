package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kinds of suite input. The kind and the evals-dir-relative name together
// frame each part of the hash, so a file cannot be confused with a different
// kind of input and a missing file cannot be confused with an empty one.
const (
	suiteKindRubric  = "rubric"
	suiteKindCase    = "case"
	suiteKindFile    = "file"
	suiteKindFileX   = "file+x" // a file with an execute bit (copyTree preserves it)
	suiteKindMissing = "missing"
)

// suiteInput is one part of an eval suite's identity.
type suiteInput struct {
	kind string
	name string // slash-separated, relative to the evals directory
	data []byte
}

// suiteSHA256 returns the lowercase hex SHA-256 of agent's eval suite: its
// rubric file(s), its case files, and every workspace fixture file, hidden
// (inject) file, mock script and setup file those cases reference. It reads
// the active evals directory (--evals-dir) and names every input relative to
// it, so the result does not depend on where that directory lives.
//
// The agent config, prompt and resources (that is prompt_sha256), models, the
// files of other agents and workspace fixtures no case references are not part
// of the suite.
func suiteSHA256(agent string) (string, error) {
	inputs, err := collectSuiteInputs(agent)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writeHashPart(h, "suite", []byte("v1")) // domain separation from prompt_sha256
	for _, in := range inputs {
		writeHashPart(h, in.kind+" "+in.name, in.data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// collectSuiteInputs gathers the de-duplicated inputs of agent's suite, sorted
// by (kind, name). A referenced file that does not exist is recorded as a
// "missing" input rather than an error: the case reports the problem when it
// runs, and the suite identity must still be stable. Any other read error is
// returned.
func collectSuiteInputs(agent string) ([]suiteInput, error) {
	c := &suiteCollector{seen: map[string]bool{}}

	if err := c.rubrics(agent); err != nil {
		return nil, err
	}
	if err := c.caseFiles(agent); err != nil {
		return nil, err
	}

	cases, err := loadCases(agent)
	if err != nil {
		return nil, err
	}
	for _, tc := range cases {
		if tc.Workspace != "" {
			if err := c.workspace(tc.Workspace); err != nil {
				return nil, err
			}
		}
		for _, chk := range tc.Checks {
			for _, inj := range chk.Inject {
				if err := c.file(path2slash(filepath.Join("fixtures", "hidden", inj)), evalsPath("fixtures", "hidden", inj)); err != nil {
					return nil, err
				}
			}
		}
		for _, m := range tc.Mocks {
			if err := c.file(path2slash(m.Script), evalsPath(m.Script)); err != nil {
				return nil, err
			}
		}
		for _, s := range tc.Setup {
			if s.Type != "file" || s.Path == "" {
				continue
			}
			p := rebaseEvalsPath(s.Path)
			if err := c.file(c.setupName(s.Path, p), p); err != nil {
				return nil, err
			}
		}
	}

	sort.Slice(c.inputs, func(i, j int) bool {
		a, b := c.inputs[i], c.inputs[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.name < b.name
	})
	return c.inputs, nil
}

type suiteCollector struct {
	inputs []suiteInput
	seen   map[string]bool
}

func path2slash(p string) string { return filepath.ToSlash(filepath.Clean(p)) }

// add records an input once per (kind, name).
func (c *suiteCollector) add(kind, name string, data []byte) {
	key := kind + "\x00" + name
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.inputs = append(c.inputs, suiteInput{kind: kind, name: name, data: data})
}

// rubrics adds every rubrics/*.yaml whose parsed agent equals agent, the same
// selection loadRubrics makes.
func (c *suiteCollector) rubrics(agent string) error {
	dir := evalsPath("rubrics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read rubrics directory: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("failed to read rubric %s: %w", e.Name(), err)
		}
		var head struct {
			Agent string `yaml:"agent"`
		}
		if err := yaml.Unmarshal(data, &head); err != nil {
			return fmt.Errorf("failed to parse rubric %s: %w", e.Name(), err)
		}
		if head.Agent == agent {
			c.add(suiteKindRubric, "rubrics/"+e.Name(), data)
		}
	}
	return nil
}

// caseFiles adds every cases/<agent>/*.yaml (non-recursive, as loadCases
// reads them), whatever --case selects.
func (c *suiteCollector) caseFiles(agent string) error {
	dir := evalsPath("cases", agent)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read cases directory for %s: %w", agent, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("failed to read case %s: %w", e.Name(), err)
		}
		c.add(suiteKindCase, "cases/"+agent+"/"+e.Name(), data)
	}
	return nil
}

// workspace adds every regular file under fixtures/workspaces/<name>.
// Symlinks and other special files are skipped, like copyTree. The execute bit
// is part of the identity because copyTree preserves it.
func (c *suiteCollector) workspace(name string) error {
	root := workspaceFixtureDir(name)
	relRoot := path2slash(filepath.Join("fixtures", "workspaces", name))
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		c.add(suiteKindMissing, relRoot, nil)
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		return c.file(relRoot+"/"+filepath.ToSlash(rel), p)
	})
}

// file reads p and adds it as a "file" (or "file+x") input named name. A file
// that does not exist is added as "missing".
func (c *suiteCollector) file(name, p string) error {
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			c.add(suiteKindMissing, name, nil)
			return nil
		}
		return fmt.Errorf("suite input %s: %w", name, err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("suite input %s: %w", name, err)
	}
	kind := suiteKindFile
	if info.Mode().Perm()&0o111 != 0 {
		kind = suiteKindFileX
	}
	c.add(kind, name, data)
	return nil
}

// setupName is the logical name of a setup file: its path relative to the
// evals directory, or its cleaned path as written when it lies outside it.
func (c *suiteCollector) setupName(written, resolved string) string {
	if rel, err := filepath.Rel(cfg.evalsDir, resolved); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return path2slash(written)
}
