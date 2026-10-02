# Issue: Add status command to TUI

## Problem
Users need a way to see which agents are currently running and what issues they're working on.

## User Story
As a user, I want to run a "status" command in the REPL to see:
- Which issues have active agents
- How long each agent has been running
- The current status of each agent

## Acceptance Criteria
- [ ] `status` command shows all active agents
- [ ] Output includes issue number, title (truncated), status, and elapsed time
- [ ] Command shows "No agents running" when none are active
- [ ] Table formatting is clean and readable

## Context
This is a simple enhancement to improve user visibility into system state.
