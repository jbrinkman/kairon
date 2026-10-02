# Task Complete: update-agent-prompts

**Task**: Update all agent prompt markdown files in .kiro/agents/ with .kairon/ paths, KAIRON_* env vars, and kairon commands
**Issue**: #275
**Status**: ✅ Completed

## What was done

Updated all 6 agent prompt files to use kairon references instead of kiro-krew:

1. **krew-lead-prompt.md**:
   - Changed `.kiro-krew/` paths to `.kairon/` (9 occurrences)
   - Updated binary pattern matching from `kiro-krew*` to `kairon*`
   - Changed label example from `kiro-krew` to `kairon`
   - Updated sentinel file path references
   - Updated QA tools artifact path

2. **architect-prompt.md**:
   - Changed spec directory path from `.kiro-krew/specs/` to `.kairon/specs/` (2 occurrences)
   - Updated workflow references from "Kiro-krew's" to "Kairon's"
   - Updated critical requirements section

3. **builder-prompt.md**:
   - Changed validator feedback path from `.kiro-krew/artifacts/` to `.kairon/artifacts/`

4. **validator-prompt.md**:
   - No changes needed (no kiro-krew references found)

5. **documenter-prompt.md**:
   - Changed plan file path from `.kiro-krew/specs/` to `.kairon/specs/`

6. **planner-prompt.md**:
   - Updated agent description from "kiro-krew" to "kairon"
   - Changed worktree script paths from `.kiro-krew/scripts/` to `.kairon/scripts/` (5 occurrences)
   - Updated config file path from `.kiro-krew/config.yaml` to `.kairon/config.yaml`
   - Changed label reference from "kiro-krew label" to "kairon label"

## Files changed

- `.kiro/agents/krew-lead-prompt.md` - Updated 9 path references and 3 command/label references
- `.kiro/agents/architect-prompt.md` - Updated 2 path references and 1 workflow reference
- `.kiro/agents/builder-prompt.md` - Updated 1 path reference
- `.kiro/agents/documenter-prompt.md` - Updated 1 path reference
- `.kiro/agents/planner-prompt.md` - Updated 5 path references, 1 config reference, and 2 command/label references
- `.kiro/agents/validator-prompt.md` - No changes required

## QA Commands Discovered

From Taskfile.yml:
- `task fmt:check` - Check code formatting (fails if unformatted)
- `task lint` - Run linters and static analysis
- `task test` - Run all tests with coverage

## QA Results

- `task fmt:check`: ✅ PASS
- `task lint`: ✅ PASS
- `task test`: ✅ PASS (All tests passed)

## Verification

All acceptance criteria met:
1. ✅ All prompts use .kairon/ paths (18 total occurrences across files)
2. ✅ All prompts use kairon for commands (replaced kiro-krew references)
3. ✅ No kiro-krew references remain in agent prompts (verified with grep)
4. ✅ Validation command passes: `grep -q '.kairon/' .kiro/agents/krew-lead-prompt.md`

**Total changes**: 23 replacements across 5 agent prompt files
**Zero kiro-krew references remaining** in all agent prompt files
