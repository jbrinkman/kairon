// Package checks is the deterministic check engine: the declarative pass/fail
// checks (command, file_*, changed_files, output_*, gh_log_*, judge, fact_*),
// their validation and their evaluation against a directory and some evidence.
//
// It looks only at an Input (a directory, an output, a gh log, ...). It does
// not know about eval cases, rubrics, sandboxes or the eval runner, and it
// must not import internal/eval or internal/eval/sandbox (TestNoHarnessDeps
// enforces this), so the workflow engine and `kairon check` can reuse it
// without linking the harness.
package checks
