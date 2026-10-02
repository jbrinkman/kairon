# Contributing to Kairon

Thank you for your interest in contributing to Kairon! This document outlines the process for contributing to the [jbrinkman/kairon](https://github.com/jbrinkman/kairon) project.

## Philosophy: Discussion Before Code

We follow a **discussion-before-coding** philosophy. Before writing any code, open an issue to discuss your proposed change. This prevents wasted effort on features that may not align with project goals or are already being worked on.

## PR-to-Issue Linking Policy

**All pull requests must be linked to an issue.** PRs that do not meet the following criteria will be automatically closed:

1. The linked issue must be labeled **"accepted"** OR assigned to you
2. If an issue is assigned to someone else, PRs from unassigned contributors will be closed

This policy ensures that all contributions are coordinated and expected.

## Getting an Issue Accepted

1. **Open an issue** describing the problem or feature you'd like to work on
2. **Participate in discussion** — maintainers may ask clarifying questions or suggest alternative approaches
3. **Wait for the "accepted" label** or assignment before starting work
4. Once accepted, you may open a PR referencing that issue

## Submitting a Pull Request

1. Fork the repository and create a branch from `main`
2. Reference the issue in your PR description (e.g., `Closes #123`)
3. Keep changes focused — one issue per PR
4. Ensure all tests pass and linting is clean
5. Include tests for new functionality
6. Update documentation if applicable

## Quality Requirements

- Code must pass all existing tests
- New features should include appropriate test coverage
- Follow the existing code style and conventions
- Commit messages should be clear and descriptive
- PRs should be reasonably scoped — avoid large, unfocused changesets

## Code of Conduct

Be respectful and constructive in all interactions. We are all here to build something great together.

## Questions?

If you're unsure about anything, open an issue to ask. We'd rather answer questions early than review a PR that can't be accepted.

## Development Setup

To set up Kairon for development:

1. Clone the repository:
   ```bash
   git clone https://github.com/jbrinkman/kairon.git
   cd kairon
   ```

2. Build the project:
   ```bash
   task build
   # or
   go build ./cmd/kairon
   ```

3. Run tests:
   ```bash
   task test
   ```

4. The project structure includes:
   - `.kairon/` — configuration and runtime files
   - Agent configurations in `.kiro/agents/`
   - Build automation via [Task](https://taskfile.dev)

For more details on using Kairon, see the [README](README.md).
