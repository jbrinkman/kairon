#!/usr/bin/env bash
#
# eval-planner.sh - run the planner eval suite against both planner agents.
#
# The planner suite (.kairon/evals/cases/planner) is keyed by the directory
# name, so every case runs as the agent "planner". The planner is split in two
# single-mode agents, so each half of the suite is run against the agent of its
# mode:
#
#   interactive half  cases: interactive-single-decision, interactive-option-rules,
#                            interactive-return-document, multi-issue-split
#                     agent: planner (.kiro/agents/planner.json), unchanged
#
#   automated half    cases: automated-best-effort-fill, automated-bug-classification,
#                            automated-punch-out
#                     agent: planner-automated, run through an --evals-dir overlay
#                            whose agents/planner.json is the planner-automated
#                            manifest with only "name" changed to "planner", plus
#                            the planner-automated prompt next to it. The shared
#                            skill is NOT copied: the eval harness stages the
#                            tracked .kiro/skills/planner-shared from the repo.
#
# Always runs with --sandbox: an agent that is not yet correct can run
# `gh issue create`, and outside the sandbox that creates real GitHub issues.
#
# Usage:
#   scripts/eval-planner.sh            run both halves (needs a model backend)
#   scripts/eval-planner.sh --dry-run  print the commands and the overlay; run nothing,
#                                      write nothing
#
# Environment (all optional):
#   KAIRON_BIN         kairon binary to run (default: kairon from PATH)
#   KAIRON_EVALS_DIR   evals directory (default: .kairon/evals)
#
# Exit status: 0 only if both halves pass. Both halves always run, and the new
# result directories of the automated half are copied back under
# <evals-dir>/results/ whether or not it passed.

set -u

INTERACTIVE_CASES='interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split'
AUTOMATED_CASES='automated-best-effort-fill,automated-bug-classification,automated-punch-out'
INTERACTIVE_FILTER="name==${INTERACTIVE_CASES}"
AUTOMATED_FILTER="name==${AUTOMATED_CASES}"

AUTOMATED_MANIFEST='.kiro/agents/planner-automated.json'
AUTOMATED_PROMPT='.kiro/agents/planner-automated-prompt.md'

usage() {
  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
}

dry_run=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry_run=1 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "eval-planner.sh: unknown argument: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

# Run from the repository root so relative paths (.kiro, .kairon) resolve.
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
cd "$repo_root" || exit 1

evals_dir="${KAIRON_EVALS_DIR:-.kairon/evals}"
kairon_bin="${KAIRON_BIN:-kairon}"

# rename_manifest copies the planner-automated manifest to stdout with only the
# top-level "name" value changed to "planner". sed -E works on macOS and Linux.
rename_manifest() {
  sed -E 's/^([[:space:]]*)"name":[[:space:]]*"planner-automated"/\1"name": "planner"/' "$AUTOMATED_MANIFEST"
}

# The interactive command uses the default evals directory unless one is given.
interactive_extra=''
if [ -n "${KAIRON_EVALS_DIR:-}" ]; then
  interactive_extra="--evals-dir ${evals_dir} "
fi

if [ "$dry_run" -eq 1 ]; then
  overlay='<overlay-dir>'
  echo "Dry run: nothing is executed and nothing is written under ${evals_dir}/."
  echo
  echo "Interactive half (agent: planner, unchanged):"
  echo "  kairon eval ${interactive_extra}--sandbox planner --filter '${INTERACTIVE_FILTER}'"
  echo
  echo "Automated half (agent: planner-automated, run as 'planner' through an overlay):"
  echo "  overlay ${overlay} = copy of ${evals_dir} without results/, plus:"
  echo "    ${overlay}/agents/planner.json  <- ${AUTOMATED_MANIFEST} with only name changed:"
  rename_manifest | grep -E '^[[:space:]]*"name":' | sed 's/^/        /'
  echo "    ${overlay}/agents/planner-automated-prompt.md  <- ${AUTOMATED_PROMPT}"
  echo "  The shared skill is not copied: .kiro/skills/planner-shared is staged from the repository."
  echo "  kairon eval --evals-dir ${overlay} --sandbox planner --filter '${AUTOMATED_FILTER}'"
  echo
  echo "New result directories of the automated half are copied to ${evals_dir}/results/."
  echo "The script exits non-zero if either half fails."
  exit 0
fi

if ! command -v "$kairon_bin" >/dev/null 2>&1; then
  echo "eval-planner.sh: '$kairon_bin' not found; build it (task build) and put it on PATH, or set KAIRON_BIN" >&2
  exit 2
fi
for f in "$AUTOMATED_MANIFEST" "$AUTOMATED_PROMPT" .kiro/skills/planner-shared/SKILL.md; do
  if [ ! -f "$f" ]; then
    echo "eval-planner.sh: missing $f" >&2
    exit 2
  fi
done
if [ ! -d "$evals_dir" ]; then
  echo "eval-planner.sh: evals directory not found: $evals_dir" >&2
  exit 2
fi

overlay=$(mktemp -d "${TMPDIR:-/tmp}/kairon-eval-planner.XXXXXX") || exit 1
trap 'rm -rf "$overlay"' EXIT

fail=0

echo "== Interactive half: planner =="
# shellcheck disable=SC2086 # interactive_extra is intentionally word-split
"$kairon_bin" eval ${interactive_extra}--sandbox planner --filter "$INTERACTIVE_FILTER"
rc=$?
if [ "$rc" -ne 0 ]; then
  echo "eval-planner.sh: interactive half failed (exit $rc)" >&2
  fail=1
fi

echo
echo "== Automated half: planner-automated (as 'planner' through an overlay) =="
# Copy the evals directory without results/ so only this run's results are new.
find "$evals_dir" -mindepth 1 -maxdepth 1 ! -name results ! -name tmp ! -name agents -exec cp -R {} "$overlay"/ \;
mkdir -p "$overlay/agents"
if [ -d "$evals_dir/agents" ]; then
  cp -R "$evals_dir/agents/." "$overlay/agents/"
fi
rename_manifest > "$overlay/agents/planner.json"
cp "$AUTOMATED_PROMPT" "$overlay/agents/planner-automated-prompt.md"

"$kairon_bin" eval --evals-dir "$overlay" --sandbox planner --filter "$AUTOMATED_FILTER"
rc=$?
if [ "$rc" -ne 0 ]; then
  echo "eval-planner.sh: automated half failed (exit $rc)" >&2
  fail=1
fi

# Copy new result directories back, pass or fail: a failing run is still evidence.
if [ -d "$overlay/results" ]; then
  mkdir -p "$evals_dir/results"
  for d in "$overlay"/results/*/; do
    [ -d "$d" ] || continue
    name=$(basename "$d")
    if [ -e "$evals_dir/results/$name" ]; then
      echo "eval-planner.sh: results/$name already exists, not overwriting" >&2
      fail=1
      continue
    fi
    cp -R "$d" "$evals_dir/results/$name"
    echo "Copied results/$name to $evals_dir/results/"
  done
fi

if [ "$fail" -ne 0 ]; then
  echo "eval-planner.sh: FAILED" >&2
  exit 1
fi
echo "eval-planner.sh: both halves passed"
