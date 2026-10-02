#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

if [ $# -eq 0 ]; then
    echo -e "${RED}Error: Planning worktree path required${NC}" >&2
    echo "Usage: $0 <worktree-path>" >&2
    echo "Example: $0 .worktrees/planning-20240628-141549" >&2
    exit 1
fi

WORKTREE_PATH=$1
PLANNING_NAME=$(basename "$WORKTREE_PATH")
BRANCH_NAME="planning/${PLANNING_NAME}"

echo -e "${BLUE}Cleaning up planning worktree ${PLANNING_NAME}...${NC}" >&2

# Validate this is actually a planning worktree
if [[ ! "$PLANNING_NAME" =~ ^planning-[0-9]{8}-[0-9]{6}$ ]]; then
    echo -e "${RED}Error: Not a valid planning worktree name: ${PLANNING_NAME}${NC}" >&2
    echo -e "${RED}Expected format: planning-YYYYMMDD-HHMMSS${NC}" >&2
    exit 1
fi

# Remove worktree if it exists
if [ -d "$WORKTREE_PATH" ] && git worktree list | grep -q "$WORKTREE_PATH"; then
    git worktree remove "$WORKTREE_PATH" --force 2>/dev/null
    echo -e "${GREEN}✓ Planning worktree removed${NC}" >&2
fi

# Delete planning branch if it exists
if git branch --list "$BRANCH_NAME" | grep -q "$BRANCH_NAME"; then
    git branch -D "$BRANCH_NAME" 2>/dev/null
    echo -e "${GREEN}✓ Planning branch ${BRANCH_NAME} deleted${NC}" >&2
fi

# Prune stale worktree entries
git worktree prune 2>/dev/null
echo -e "${GREEN}✓ Worktree entries pruned${NC}" >&2

echo -e "${GREEN}✓ Planning worktree cleanup complete${NC}" >&2
