package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// auditThresholdPercent is the minimum share of instructions that must be
// load-bearing (mapped to a rubric criterion) for an audit to pass.
const auditThresholdPercent = 90

// auditMinCriteria is the minimum number of distinct non-cost criteria a
// rubric must carry (Stage 3 requires at least three).
const auditMinCriteria = 3

// auditSkillsDisplayDir is where shipped engine skills live in the source tree;
// it is only used in messages and file labels.
const auditSkillsDisplayDir = "cmd/kairon/templates/kiro/skills"

// auditConventionsSuffix marks a skill as a per-project override hook.
const auditConventionsSuffix = "-conventions"

// Audit rule IDs. They are stable and documented in docs/evaluation.md.
const (
	auditRuleStructure        = "structure"
	auditRuleResource         = "resource"
	auditRuleTable            = "table"
	auditRuleDuplicate        = "duplicate"
	auditRuleUnmapped         = "unmapped"
	auditRuleUnknownCriterion = "unknown-criterion"
	auditRuleCoverage         = "coverage"
	auditRuleRubric           = "rubric"
	auditRuleUnchecked        = "unchecked"
	auditRuleIdentical        = "identical"
)

// auditRuleOrder ranks rules for stable output ordering within a file and line.
var auditRuleOrder = map[string]int{
	auditRuleStructure: 0, auditRuleResource: 1, auditRuleTable: 2, auditRuleDuplicate: 3,
	auditRuleUnmapped: 4, auditRuleUnknownCriterion: 5, auditRuleCoverage: 6,
	auditRuleRubric: 7, auditRuleUnchecked: 8, auditRuleIdentical: 9,
}

// rtccHeadings are the level-2 headings a prompt must carry, in this order.
var rtccHeadings = []string{"Role", "Task", "Context", "Constraints"}

var (
	// atxHeadingRe matches an ATX heading of any level (up to 3 leading
	// spaces; 4+ is an indented code block).
	atxHeadingRe = regexp.MustCompile(`^ {0,3}#{1,6}([ \t]|$)`)
	// tableSeparatorRe matches a table separator row: only pipes, dashes,
	// colons and spaces.
	tableSeparatorRe = regexp.MustCompile(`^[|:\- \t]+$`)
	// auditEvidenceRe finds "case:<name>" tokens in an Evidence cell. The
	// leading boundary keeps "showcase:x" from matching; trailing '.' and '-'
	// are stripped from the capture (sentence punctuation).
	auditEvidenceRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])case:([A-Za-z0-9_.-]+)`)
)

// AuditOptions configures Audit.
type AuditOptions struct {
	IterationsDir string // "" => DefaultIterationsDir
	EvalsDir      string // "" => defaultEvalsDir
	PromptFile    string // "" => the agent's own prompt; else audit this candidate instead
	// SkillsFS holds the shipped engine skills, rooted so "<name>/SKILL.md"
	// resolves. nil means no engine skill can be resolved.
	SkillsFS fs.FS
}

// AuditViolation is one failed audit rule, rendered
// "<file>[:<line>]: <rule>: <message>".
type AuditViolation struct {
	File    string
	Line    int // 0 => no line
	Rule    string
	Message string
}

// String renders the violation.
func (v AuditViolation) String() string {
	if v.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", v.File, v.Line, v.Rule, v.Message)
	}
	return fmt.Sprintf("%s: %s: %s", v.File, v.Rule, v.Message)
}

// AuditError carries every violation found by Audit.
type AuditError struct{ Violations []AuditViolation }

// Error renders one line per violation.
func (e *AuditError) Error() string {
	lines := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		lines[i] = v.String()
	}
	return strings.Join(lines, "\n")
}

// auditInstruction is one counted line of a prompt or skill.
type auditInstruction struct {
	File string
	Line int // 1-based
	Text string
}

// auditSkill is an engine skill that was read and is scored.
type auditSkill struct {
	name         string
	instructions []auditInstruction
}

// auditTableRow is one parsed data row of a load-bearing table.
type auditTableRow struct {
	criterion string
	line      int
}

// Audit checks an agent's prompt for RTCC structure and for load-bearing
// coverage: the share of its instructions that the agent's load-bearing table
// maps to a rubric criterion. It is stateless (directories are explicit, no
// package state is read) and makes no model calls.
//
// The report goes to w. Rule failures are returned as *AuditError; anything
// that prevents auditing at all (bad agent name, missing config, unreadable
// candidate, missing or empty rubric) is a plain error.
func Audit(w io.Writer, agent string, opts AuditOptions) error {
	if err := validatePathElement("agent name", agent); err != nil {
		return err
	}
	itersDir := opts.IterationsDir
	if itersDir == "" {
		itersDir = DefaultIterationsDir
	}
	evalsDir := opts.EvalsDir
	if evalsDir == "" {
		evalsDir = defaultEvalsDir
	}

	configPath, conf, err := loadAuditConfig(evalsDir, agent)
	if err != nil {
		return err
	}
	promptPath, promptText, isCandidate, err := loadAuditPrompt(agent, configPath, conf, opts.PromptFile)
	if err != nil {
		return err
	}
	rubricPath := filepath.Join(evalsDir, "rubrics", agent+".yaml")
	criteria, err := loadAuditCriteria(rubricPath)
	if err != nil {
		return err
	}
	criterionNames := auditCriterionNames(criteria)
	cases, err := loadAuditCases(evalsDir, agent)
	if err != nil {
		return err
	}
	tablePath := filepath.Join(itersDir, agent, "load-bearing.md")

	// Files are ranked by registration order for stable output: the prompt,
	// then skills, then the table, then the config.
	rank := map[string]int{}
	register := func(file string) {
		if _, ok := rank[file]; !ok {
			rank[file] = len(rank)
		}
	}
	var violations []AuditViolation
	add := func(file string, line int, rule, format string, args ...any) {
		violations = append(violations, AuditViolation{File: file, Line: line, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}

	register(promptPath)
	for _, msg := range checkRTCC(promptText) {
		add(promptPath, 0, auditRuleStructure, "%s", msg)
	}
	instructions := extractInstructions(promptPath, promptText)

	skills, hooks, ignored, resourceProblems := resolveAuditResources(conf, opts.SkillsFS)
	for _, s := range skills {
		file := path.Join(auditSkillsDisplayDir, s.name, "SKILL.md")
		register(file)
		instructions = append(instructions, s.instructions...)
	}
	register(tablePath)
	register(rubricPath)
	register(configPath)
	for _, msg := range resourceProblems {
		add(configPath, 0, auditRuleResource, "%s", msg)
	}

	table := loadAuditTable(tablePath, agent, criterionNames, cases, add)

	// Rubric-level rules: always reported, independent of coverage.
	criterionStats := checkAuditRubric(add, rubricPath, criteria, cases)

	// A later instruction whose text equals an earlier one (prompt first, then
	// engine skills) is dead weight. Every repeat refers to the first
	// occurrence. Always reported, regardless of coverage; counting is unchanged.
	duplicates := 0
	first := make(map[string]auditInstruction, len(instructions))
	for _, in := range instructions {
		if prev, seen := first[in.Text]; seen {
			duplicates++
			add(in.File, in.Line, auditRuleDuplicate, "duplicate of %s:%d: %q", prev.File, prev.Line, in.Text)
			continue
		}
		first[in.Text] = in
	}

	// Map every instruction. Per-instruction findings are details of a
	// coverage failure: they are reported only when coverage is below the
	// threshold, so a prompt at or above it can still carry unmapped lines.
	mapped := 0
	var details []AuditViolation
	for _, in := range instructions {
		row, ok := table[in.Text]
		switch {
		case !ok:
			details = append(details, AuditViolation{File: in.File, Line: in.Line, Rule: auditRuleUnmapped,
				Message: fmt.Sprintf("instruction not in load-bearing table: %q", in.Text)})
		case !criterionNames[row.criterion]:
			details = append(details, AuditViolation{File: in.File, Line: in.Line, Rule: auditRuleUnknownCriterion,
				Message: fmt.Sprintf("maps to criterion %q which is not in rubric %s: %q", row.criterion, rubricPath, in.Text)})
		default:
			mapped++
		}
	}
	total := len(instructions)
	percent := auditPercent(mapped, total)
	if total > 0 && mapped*100 >= auditThresholdPercent*total {
		// pass: details are not violations
	} else {
		violations = append(violations, details...)
		if total == 0 {
			add(tablePath, 0, auditRuleCoverage, "no instructions found to audit; need at least %d%% to be load-bearing", auditThresholdPercent)
		} else {
			add(tablePath, 0, auditRuleCoverage, "%s of instructions are load-bearing; need at least %d%%", percent, auditThresholdPercent)
		}
	}

	if err := writeAuditReport(w, agent, promptPath, isCandidate, skills, hooks, ignored, total, mapped, duplicates, percent, criterionStats); err != nil {
		return err
	}

	if len(violations) > 0 {
		sort.SliceStable(violations, func(i, j int) bool {
			a, b := violations[i], violations[j]
			if rank[a.File] != rank[b.File] {
				return rank[a.File] < rank[b.File]
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return auditRuleOrder[a.Rule] < auditRuleOrder[b.Rule]
		})
		return &AuditError{Violations: violations}
	}
	_, err = fmt.Fprintf(w, "OK %s: %d instructions, %s load-bearing, RTCC structure present\n", agent, total, percent)
	return err
}

// auditPercent formats mapped/total as a percentage truncated to one decimal,
// using integer arithmetic so it can never round up across the threshold.
func auditPercent(mapped, total int) string {
	if total == 0 {
		return "0.0%"
	}
	tenths := mapped * 1000 / total
	return fmt.Sprintf("%d.%d%%", tenths/10, tenths%10)
}

func writeAuditReport(w io.Writer, agent, promptPath string, isCandidate bool, skills []auditSkill, hooks, ignored []string, total, mapped, duplicates int, percent string, stats []auditCriterionStat) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Audit: %s\n", agent)
	if isCandidate {
		fmt.Fprintf(&b, "Prompt: %s (candidate)\n", promptPath)
	} else {
		fmt.Fprintf(&b, "Prompt: %s\n", promptPath)
	}
	if len(skills) > 0 {
		b.WriteString("Audited resources:\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "  skill %s (%d instructions)\n", s.name, len(s.instructions))
		}
	}
	if len(hooks) > 0 {
		b.WriteString("Override hooks (excluded from score):\n")
		for _, h := range hooks {
			fmt.Fprintf(&b, "  skill %s\n", h)
		}
	}
	if len(ignored) > 0 {
		b.WriteString("Ignored resources:\n")
		for _, r := range ignored {
			fmt.Fprintf(&b, "  %s\n", r)
		}
	}
	fmt.Fprintf(&b, "Instructions: total=%d mapped=%d duplicates=%d coverage=%s (threshold %d%%)\n", total, mapped, duplicates, percent, auditThresholdPercent)
	fmt.Fprintf(&b, "Criteria: non-cost=%d (need at least %d)\n", len(stats), auditMinCriteria)
	for _, st := range stats {
		fmt.Fprintf(&b, "  %s: checks=%d cases=%d\n", st.name, st.checks, st.cases)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// loadAuditConfig locates and decodes the agent config: the evals-dir overlay
// wins over .kiro/agents. It does not use package state.
func loadAuditConfig(evalsDir, agent string) (string, agentConfigFile, error) {
	overlay := filepath.Join(evalsDir, "agents", agent+".json")
	project := filepath.Join(".kiro", "agents", agent+".json")
	configPath := overlay
	if _, err := os.Stat(overlay); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", agentConfigFile{}, fmt.Errorf("agent config %s: %w", overlay, err)
		}
		if _, err := os.Stat(project); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return "", agentConfigFile{}, fmt.Errorf("agent config %s: %w", project, err)
			}
			return "", agentConfigFile{}, fmt.Errorf("agent config for %q not found: looked for %s and %s", agent, overlay, project)
		}
		configPath = project
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", agentConfigFile{}, fmt.Errorf("reading agent config %s: %w", configPath, err)
	}
	var conf agentConfigFile
	if err := json.Unmarshal(data, &conf); err != nil {
		return "", agentConfigFile{}, fmt.Errorf("parsing agent config %s: %w", configPath, err)
	}
	return configPath, conf, nil
}

// loadAuditPrompt returns the text to audit: the candidate when promptFile is
// set (validated like any --prompt-file), else the file:// prompt of the config.
func loadAuditPrompt(agent, configPath string, conf agentConfigFile, promptFile string) (displayPath, text string, candidate bool, err error) {
	if promptFile != "" {
		cand, err := loadCandidatePrompt(agent, RunOptions{PromptFile: promptFile})
		if err != nil {
			return "", "", false, err
		}
		return cand.Path, string(cand.Content), true, nil
	}
	ref, ok := strings.CutPrefix(conf.Prompt, "file://")
	if !ok {
		return "", "", false, fmt.Errorf("agent config %s has an inline prompt; only a file:// prompt can be audited", configPath)
	}
	if ref == "" {
		return "", "", false, fmt.Errorf("agent config %s has an empty file:// prompt reference", configPath)
	}
	if filepath.IsAbs(ref) || path.IsAbs(filepath.ToSlash(ref)) {
		return "", "", false, fmt.Errorf("agent config %s references an absolute prompt path %q; only a path relative to the config can be audited", configPath, ref)
	}
	p := filepath.Join(filepath.Dir(configPath), filepath.FromSlash(ref))
	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", false, fmt.Errorf("reading prompt %s: %w", p, err)
	}
	return p, string(data), false, nil
}

// auditCriterion is one rubric criterion as the audit sees it.
type auditCriterion struct {
	name string
	cost bool // type: cost
}

// loadAuditCriteria returns the criteria of a rubric file in rubric order,
// de-duplicated by name (first occurrence wins). Criteria with an empty name
// are ignored. A criterion is a cost criterion when its type is exactly
// "cost", as the runner treats it.
func loadAuditCriteria(rubricPath string) ([]auditCriterion, error) {
	data, err := os.ReadFile(rubricPath)
	if err != nil {
		return nil, fmt.Errorf("reading rubric %s: %w", rubricPath, err)
	}
	var r Rubric
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing rubric %s: %w", rubricPath, err)
	}
	if len(r.Criteria) == 0 {
		return nil, fmt.Errorf("rubric %s has no criteria", rubricPath)
	}
	seen := make(map[string]bool, len(r.Criteria))
	criteria := make([]auditCriterion, 0, len(r.Criteria))
	for _, c := range r.Criteria {
		name := strings.TrimSpace(c.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		criteria = append(criteria, auditCriterion{name: name, cost: strings.TrimSpace(c.Type) == "cost"})
	}
	return criteria, nil
}

// auditCriterionNames returns the set of criterion names (cost included).
func auditCriterionNames(criteria []auditCriterion) map[string]bool {
	names := make(map[string]bool, len(criteria))
	for _, c := range criteria {
		names[c.name] = true
	}
	return names
}

// resolveAuditResources classifies the config's resources. Engine skills are
// read from skillsFS; override hooks (*-conventions) are only listed. Each
// skill is processed once, in first-occurrence order. Anything that is not a
// skill:// string is reported as ignored.
func resolveAuditResources(conf agentConfigFile, skillsFS fs.FS) (skills []auditSkill, hooks, ignored, problems []string) {
	seen := map[string]bool{}
	for _, raw := range conf.Resources {
		var uri string
		if json.Unmarshal(raw, &uri) != nil || !strings.HasPrefix(uri, "skill://") {
			if uri == "" {
				uri = strings.TrimSpace(string(raw))
			}
			ignored = append(ignored, uri)
			continue
		}
		name := path.Base(path.Dir(strings.TrimPrefix(uri, "skill://")))
		if validatePathElement("skill name", name) != nil || name == "." || strings.ContainsAny(name, "*?[") {
			if !seen[uri] {
				seen[uri] = true
				problems = append(problems, fmt.Sprintf("resource %q does not name a single skill directory and cannot be audited", uri))
			}
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if strings.HasSuffix(name, auditConventionsSuffix) {
			hooks = append(hooks, name)
			continue
		}
		var data []byte
		var err error = fs.ErrNotExist
		if skillsFS != nil {
			data, err = fs.ReadFile(skillsFS, path.Join(name, "SKILL.md"))
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("skill %q is not shipped under %s/ and cannot be audited", name, auditSkillsDisplayDir))
			continue
		}
		file := path.Join(auditSkillsDisplayDir, name, "SKILL.md")
		skills = append(skills, auditSkill{name: name, instructions: extractInstructions(file, string(data))})
	}
	return skills, hooks, ignored, problems
}

// checkRTCC returns one message per RTCC problem in prompt text: each missing
// level-2 heading, and an out-of-order report if the present ones are not in
// Role, Task, Context, Constraints order. Headings in code fences do not count.
func checkRTCC(text string) []string {
	first := map[string]int{} // heading name -> line of its first occurrence
	var fence fenceTracker
	for i, line := range strings.Split(normalizeAuditText(text), "\n") {
		if fence.skip(line) {
			continue
		}
		if m := h2HeadingRe.FindStringSubmatch(line); m != nil {
			if _, seen := first[m[1]]; !seen {
				first[m[1]] = i
			}
		}
	}
	var msgs []string
	var present []string // in canonical order
	for _, h := range rtccHeadings {
		if _, ok := first[h]; !ok {
			msgs = append(msgs, fmt.Sprintf("missing required heading %q", "## "+h))
			continue
		}
		present = append(present, h)
	}
	found := append([]string(nil), present...)
	sort.SliceStable(found, func(i, j int) bool { return first[found[i]] < first[found[j]] })
	for i := range found {
		if found[i] != present[i] {
			msgs = append(msgs, fmt.Sprintf("required headings out of order: want %s; found %s",
				strings.Join(rtccHeadings, ", "), strings.Join(found, ", ")))
			break
		}
	}
	return msgs
}

// normalizeAuditText converts CRLF to LF and drops a leading BOM.
func normalizeAuditText(text string) string {
	return strings.TrimPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
}

// extractInstructions returns the instructions of a prompt or skill: every
// physical line that is non-blank, outside a code fence, not an ATX heading
// and not a table separator row, after skipping a leading YAML front-matter
// block. Each occurrence counts. Text is the trimmed line; Line is 1-based.
func extractInstructions(file, text string) []auditInstruction {
	lines := strings.Split(normalizeAuditText(text), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimRight(lines[0], " \t") == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], " \t") == "---" {
				start = i + 1
				break
			}
		}
	}
	var out []auditInstruction
	var fence fenceTracker
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if fence.skip(line) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || atxHeadingRe.MatchString(line) || isTableSeparator(trimmed) {
			continue
		}
		out = append(out, auditInstruction{File: file, Line: i + 1, Text: trimmed})
	}
	return out
}

// isTableSeparator reports whether a trimmed line is a markdown table
// separator row: only '|', '-', ':' and spaces, with at least one '-' and one '|'.
func isTableSeparator(trimmed string) bool {
	return tableSeparatorRe.MatchString(trimmed) && strings.Contains(trimmed, "-") && strings.Contains(trimmed, "|")
}

// auditCases maps a case name to its checks, grouped by criterion. Each check
// is kept as the canonical JSON of its body without the criterion field (map
// keys sorted by encoding/json), so equal checks compare equal whatever their
// YAML field order.
type auditCases struct {
	dir   string // <evals-dir>/cases/<agent>, for messages
	names map[string]map[string][]string
}

// loadAuditCases reads <evalsDir>/cases/<agent>/*.yaml (non-recursive). A case
// is identified by its name: field. Each check is decoded generically (not
// into Check) so the audit compares exactly the fields the case file carries.
// A missing directory means the agent has no cases. Like loadAuditConfig it does
// not use package state, so it cannot share loadCases.
func loadAuditCases(evalsDir, agent string) (auditCases, error) {
	dir := filepath.Join(evalsDir, "cases", agent)
	cases := auditCases{dir: dir, names: map[string]map[string][]string{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cases, nil
		}
		return auditCases{}, fmt.Errorf("reading cases directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		file := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			return auditCases{}, fmt.Errorf("reading case %s: %w", file, err)
		}
		var c struct {
			Name   string           `yaml:"name"`
			Checks []map[string]any `yaml:"checks"`
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			return auditCases{}, fmt.Errorf("parsing case %s: %w", file, err)
		}
		if c.Name == "" {
			return auditCases{}, fmt.Errorf("parsing case %s: empty name", file)
		}
		byCriterion := cases.names[c.Name]
		if byCriterion == nil {
			byCriterion = map[string][]string{}
			cases.names[c.Name] = byCriterion
		}
		for _, ch := range c.Checks {
			criterion, _ := ch["criterion"].(string)
			delete(ch, "criterion")
			byCriterion[criterion] = append(byCriterion[criterion], canonicalAuditCheck(ch))
		}
	}
	return cases, nil
}

// canonicalAuditCheck encodes a check body (criterion already removed) as JSON
// with sorted keys. A body that JSON cannot encode (non-string YAML keys) falls
// back to fmt's %v, which also prints maps in sorted key order.
func canonicalAuditCheck(body map[string]any) string {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Sprintf("%v", body)
	}
	return string(b)
}

// evidenceCaseNames returns the distinct case names an Evidence cell names via
// "case:<name>" tokens, in order of first appearance.
func evidenceCaseNames(cell string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range auditEvidenceRe.FindAllStringSubmatch(cell, -1) {
		name := strings.TrimRight(m[1], ".-")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// checkAuditEvidence reports the table violations of one data row's Evidence
// cell: no case named, a named case that does not exist, or a case with no
// check on the row's criterion (skipped when the criterion is not in the
// rubric, which is already an unknown-criterion problem).
func checkAuditEvidence(add func(file string, line int, rule, format string, args ...any), tablePath string, lineNo int, agent string, criteria map[string]bool, cases auditCases, criterion, evidence string) {
	names := evidenceCaseNames(evidence)
	if len(names) == 0 {
		add(tablePath, lineNo, auditRuleTable, "row %d: Evidence must name at least one case of agent %q as case:<case-name>", lineNo, agent)
		return
	}
	for _, name := range names {
		checked, ok := cases.names[name]
		switch {
		case !ok:
			add(tablePath, lineNo, auditRuleTable, "row %d: evidence names case %q which is not a case of agent %q (looked in %s)", lineNo, name, agent, cases.dir)
		case criteria[criterion] && len(checked[criterion]) == 0:
			add(tablePath, lineNo, auditRuleTable, "row %d: case %q has no check on criterion %q", lineNo, name, criterion)
		}
	}
}

// loadAuditTable reads and parses <iterations-dir>/<agent>/load-bearing.md into
// a map keyed by the verbatim instruction text, and validates every data row's
// Evidence cell against the agent's cases. A missing file is a table violation
// and an empty table.
func loadAuditTable(tablePath, agent string, criteria map[string]bool, cases auditCases, add func(file string, line int, rule, format string, args ...any)) map[string]auditTableRow {
	data, err := os.ReadFile(tablePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			add(tablePath, 0, auditRuleTable, "load-bearing table not found")
		} else {
			add(tablePath, 0, auditRuleTable, "cannot read load-bearing table: %v", err)
		}
		return nil
	}
	const header = "| Instruction | Criterion | Evidence |"
	table := map[string]auditTableRow{}
	headerSeen := false
	var fence fenceTracker
	for i, line := range strings.Split(normalizeAuditText(string(data)), "\n") {
		if fence.skip(line) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := splitTableRow(trimmed)
		if !headerSeen {
			if len(cells) == 3 && cells[0] == "Instruction" && cells[1] == "Criterion" && cells[2] == "Evidence" {
				headerSeen = true
			}
			continue
		}
		if isTableSeparator(trimmed) {
			continue
		}
		lineNo := i + 1
		if len(cells) != 3 {
			add(tablePath, lineNo, auditRuleTable, "row %d: expected 3 columns, got %d", lineNo, len(cells))
			continue
		}
		if cells[0] == "" || cells[1] == "" {
			add(tablePath, lineNo, auditRuleTable, "row %d: Instruction and Criterion must not be empty", lineNo)
			continue
		}
		checkAuditEvidence(add, tablePath, lineNo, agent, criteria, cases, cells[1], cells[2])
		if prev, dup := table[cells[0]]; dup {
			if prev.criterion != cells[1] {
				add(tablePath, lineNo, auditRuleTable, "instruction %q is already mapped to %q on line %d; it cannot also map to %q", cells[0], prev.criterion, prev.line, cells[1])
			}
			continue
		}
		table[cells[0]] = auditTableRow{criterion: cells[1], line: lineNo}
	}
	if !headerSeen {
		add(tablePath, 0, auditRuleTable, "no %q header row", header)
	}
	return table
}

// splitTableRow splits a trimmed row that starts with '|' into trimmed cells.
// "\|" inside a cell is an escaped pipe. A trailing pipe does not add a cell.
func splitTableRow(row string) []string {
	var cells []string
	var cur strings.Builder
	rest := row[1:] // drop the leading '|'
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == '\\' && i+1 < len(rest) && rest[i+1] == '|':
			cur.WriteByte('|')
			i++
		case rest[i] == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(rest[i])
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		cells = append(cells, strings.TrimSpace(cur.String()))
	}
	return cells
}

// auditCriterionStat is the report line of one non-cost criterion.
type auditCriterionStat struct {
	name   string
	checks int // checks on the criterion across all cases
	cases  int // distinct cases with at least one check on it
}

// checkAuditRubric applies the rubric-level rules, always reported regardless
// of coverage, all attributed to the rubric file (no line): rubric (fewer than
// auditMinCriteria distinct non-cost criteria), unchecked (a non-cost criterion
// with no check in any case) and identical (two non-cost criteria with the same
// checks in every case). Cost criteria are never counted or reported. It
// returns the per-criterion stats of the non-cost criteria in rubric order.
func checkAuditRubric(add func(file string, line int, rule, format string, args ...any), rubricPath string, criteria []auditCriterion, cases auditCases) []auditCriterionStat {
	var nonCost []string
	for _, c := range criteria {
		if !c.cost {
			nonCost = append(nonCost, c.name)
		}
	}
	if len(nonCost) < auditMinCriteria {
		list := "none"
		if len(nonCost) > 0 {
			list = strings.Join(nonCost, ", ")
		}
		add(rubricPath, 0, auditRuleRubric, "rubric %s has %d distinct non-cost criteria (%s); need at least %d", rubricPath, len(nonCost), list, auditMinCriteria)
	}

	caseNames := make([]string, 0, len(cases.names))
	for name := range cases.names {
		caseNames = append(caseNames, name)
	}
	sort.Strings(caseNames)

	stats := make([]auditCriterionStat, 0, len(nonCost))
	firstWith := map[string]string{} // signature -> first criterion carrying it
	for _, name := range nonCost {
		st := auditCriterionStat{name: name}
		var sig []string
		for _, caseName := range caseNames {
			checks := cases.names[caseName][name]
			if len(checks) == 0 {
				continue
			}
			st.checks += len(checks)
			st.cases++
			sorted := append([]string(nil), checks...)
			sort.Strings(sorted)
			sig = append(sig, caseName+"\x00"+strings.Join(sorted, "\x01"))
		}
		stats = append(stats, st)
		if st.checks == 0 {
			add(rubricPath, 0, auditRuleUnchecked, "criterion %q has no check in any case (looked in %s)", name, cases.dir)
			continue
		}
		key := strings.Join(sig, "\x02")
		if first, dup := firstWith[key]; dup {
			add(rubricPath, 0, auditRuleIdentical, "criterion %q has the same checks as criterion %q in every case (looked in %s)", name, first, cases.dir)
			continue
		}
		firstWith[key] = name
	}
	return stats
}
