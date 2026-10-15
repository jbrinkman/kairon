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
	auditRuleUnmapped         = "unmapped"
	auditRuleUnknownCriterion = "unknown-criterion"
	auditRuleCoverage         = "coverage"
)

// auditRuleOrder ranks rules for stable output ordering within a file and line.
var auditRuleOrder = map[string]int{
	auditRuleStructure: 0, auditRuleResource: 1, auditRuleTable: 2,
	auditRuleUnmapped: 3, auditRuleUnknownCriterion: 4, auditRuleCoverage: 5,
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
	register(configPath)
	for _, msg := range resourceProblems {
		add(configPath, 0, auditRuleResource, "%s", msg)
	}

	table := loadAuditTable(tablePath, add)

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
		case !criteria[row.criterion]:
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

	if err := writeAuditReport(w, agent, promptPath, isCandidate, skills, hooks, ignored, total, mapped, percent); err != nil {
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

func writeAuditReport(w io.Writer, agent, promptPath string, isCandidate bool, skills []auditSkill, hooks, ignored []string, total, mapped int, percent string) error {
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
	fmt.Fprintf(&b, "Instructions: total=%d mapped=%d coverage=%s (threshold %d%%)\n", total, mapped, percent, auditThresholdPercent)
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

// loadAuditCriteria returns the set of criterion names of a rubric file.
func loadAuditCriteria(rubricPath string) (map[string]bool, error) {
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
	names := make(map[string]bool, len(r.Criteria))
	for _, c := range r.Criteria {
		names[c.Name] = true
	}
	return names, nil
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

// loadAuditTable reads and parses <iterations-dir>/<agent>/load-bearing.md into
// a map keyed by the verbatim instruction text. A missing file is a table
// violation and an empty table.
func loadAuditTable(tablePath string, add func(file string, line int, rule, format string, args ...any)) map[string]auditTableRow {
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
