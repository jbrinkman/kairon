#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

CONFIG_FILE=".kairon/config.yaml"

# read_config_base_branch prints the top-level (column 0) base_branch value from
# the kairon config, with surrounding quotes, trailing comments and whitespace
# stripped. Prints nothing if the key is absent or empty.
read_config_base_branch() {
    [ -f "$CONFIG_FILE" ] || return 0
    sed -n 's/^base_branch:[[:space:]]*//p' "$CONFIG_FILE" 2>/dev/null \
        | head -n 1 \
        | tr -d '\r' \
        | sed -e 's/^#.*$//' \
              -e 's/[[:space:]]#.*$//' \
              -e 's/[[:space:]]*$//' \
              -e 's/^"\(.*\)"$/\1/' \
              -e "s/^'\(.*\)'\$/\1/"
}

# detect_remote_default_branch prints the default branch of the origin remote,
# using the local origin/HEAD symref first and the remote itself as a fallback.
detect_remote_default_branch() {
    local ref
    ref=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
    if [ -n "$ref" ]; then
        echo "${ref#origin/}"
        return 0
    fi
    git ls-remote --symref origin HEAD 2>/dev/null \
        | sed -n 's|^ref:[[:space:]]*refs/heads/\([^[:space:]]*\)[[:space:]]*HEAD$|\1|p' \
        | head -n 1
}

# resolve_base_branch prints the integration branch name. First non-empty wins:
#   1. $KAIRON_BASE_BRANCH
#   2. base_branch in .kairon/config.yaml
#   3. origin's default branch (origin/HEAD)
#   4. main
resolve_base_branch() {
    local base="${KAIRON_BASE_BRANCH:-}"
    if [ -z "$base" ]; then
        base=$(read_config_base_branch)
    fi
    if [ -z "$base" ] && git remote get-url origin >/dev/null 2>&1; then
        base=$(detect_remote_default_branch)
    fi
    echo "${base:-main}"
}

if [ $# -eq 0 ]; then
    echo -e "${RED}Error: Spec name required${NC}" >&2
    echo "Usage: $0 <spec-name>" >&2
    echo "Example: $0 add-auth-flow" >&2
    exit 1
fi

SPEC_NAME=$1
WORKTREE_PATH=".worktrees/${SPEC_NAME}"
BRANCH_NAME="spec/${SPEC_NAME}"

# Idempotency: if worktree already exists, print path and exit 0
if [ -d "$WORKTREE_PATH" ] && git worktree list | grep -q "$WORKTREE_PATH"; then
    echo -e "${YELLOW}Worktree already exists at ${WORKTREE_PATH}${NC}" >&2
    echo "$(pwd)/${WORKTREE_PATH}"
    exit 0
fi

echo -e "${BLUE}Creating worktree for spec ${SPEC_NAME}...${NC}" >&2

# If branch already exists but worktree doesn't, remove the stale branch
if git branch --list "$BRANCH_NAME" | grep -q "$BRANCH_NAME"; then
    echo -e "${YELLOW}Removing stale branch ${BRANCH_NAME}...${NC}" >&2
    # stdout is reserved for the worktree path; git prints "Deleted branch ..." on stdout
    git branch -D "$BRANCH_NAME" >&2
fi

# Resolve and validate the integration branch
BASE_BRANCH=$(resolve_base_branch)
CHECKED_BASE=$(git check-ref-format --branch "$BASE_BRANCH" 2>/dev/null)
if [ $? -ne 0 ] || [ "$CHECKED_BASE" != "$BASE_BRANCH" ]; then
    echo -e "${RED}Error: Invalid integration branch name: '${BASE_BRANCH}'${NC}" >&2
    exit 1
fi

# Determine the start point: the freshly fetched tip of origin/<base>
if git remote get-url origin >/dev/null 2>&1; then
    echo -e "${BLUE}Fetching origin/${BASE_BRANCH}...${NC}" >&2
    if ! git fetch origin "+refs/heads/${BASE_BRANCH}:refs/remotes/origin/${BASE_BRANCH}" --quiet >&2; then
        if git rev-parse --verify --quiet "refs/remotes/origin/${BASE_BRANCH}^{commit}" >/dev/null; then
            echo -e "${YELLOW}Warning: fetch failed; using last-known origin/${BASE_BRANCH}${NC}" >&2
        else
            echo -e "${RED}Error: fetch of origin/${BASE_BRANCH} failed and no cached origin/${BASE_BRANCH} exists${NC}" >&2
            exit 1
        fi
    fi
    if ! git rev-parse --verify --quiet "refs/remotes/origin/${BASE_BRANCH}^{commit}" >/dev/null; then
        echo -e "${RED}Error: origin/${BASE_BRANCH} not found${NC}" >&2
        exit 1
    fi
    START_POINT="origin/${BASE_BRANCH}"
else
    echo -e "${YELLOW}Warning: no 'origin' remote; branching from local HEAD${NC}" >&2
    START_POINT="HEAD"
fi

START_SHA=$(git rev-parse --short "${START_POINT}^{commit}" 2>/dev/null)
echo -e "${BLUE}Branching from ${START_POINT} (${START_SHA})${NC}" >&2

# Create the worktree on a new branch (--no-track: no upstream on spec branches)
OUTPUT=$(git worktree add --no-track "$WORKTREE_PATH" -b "$BRANCH_NAME" "$START_POINT" 2>&1)
EXIT_CODE=$?
echo "$OUTPUT" | grep -v "^$" >&2
if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ Worktree created at ${WORKTREE_PATH} on branch ${BRANCH_NAME}${NC}" >&2
    echo "$(pwd)/${WORKTREE_PATH}"
else
    echo -e "${RED}Error: Failed to create worktree for spec ${SPEC_NAME}${NC}" >&2
    exit 1
fi
