#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Generate unique planning worktree name with timestamp
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
PLANNING_NAME="planning-${TIMESTAMP}"
WORKTREE_PATH=".worktrees/${PLANNING_NAME}"
BRANCH_NAME="planning/${PLANNING_NAME}"

echo -e "${BLUE}Creating planning worktree ${PLANNING_NAME}...${NC}" >&2

# Cleanup any stale planning branches (older than 24 hours)
git for-each-ref --format='%(refname:short) %(creatordate:unix)' refs/heads/planning/ 2>/dev/null | while read -r branch_name created_time; do
    current_time=$(date +%s)
    age=$((current_time - created_time))
    # 86400 seconds = 24 hours
    if [ $age -gt 86400 ]; then
        echo -e "${YELLOW}Cleaning up stale planning branch ${branch_name}...${NC}" >&2
        git branch -D "$branch_name" 2>/dev/null
    fi
done

# Prune stale worktree entries
git worktree prune 2>/dev/null

# Create the planning worktree on a new branch
OUTPUT=$(git worktree add "$WORKTREE_PATH" -b "$BRANCH_NAME" 2>&1)
EXIT_CODE=$?
echo "$OUTPUT" | grep -v "^$" >&2

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ Planning worktree created at ${WORKTREE_PATH}${NC}" >&2
    echo "$(pwd)/${WORKTREE_PATH}"
else
    echo -e "${RED}Error: Failed to create planning worktree${NC}" >&2
    exit 1
fi
