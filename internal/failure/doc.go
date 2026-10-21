// Package failure is the uniform failure contract for pipeline stages.
//
// A failed stage ends one of two ways. Tier 1 is a bounded automatic retry.
// Tier 2 is a punch-out: an explanatory comment on the issue followed by the
// fail-state label, which removes the issue from the polling set until a human
// clears the label.
//
// This package is deterministic Go and does no I/O itself. Every GitHub effect
// goes through the Issues port, which callers implement and tests fake. To keep
// that checkable, it imports only the standard library, internal/guard,
// internal/stageoutput and internal/config, and does not directly import os,
// os/exec, net or net/http (enforced by TestNoForbiddenImports).
//
// Attempts, not retries, are the unit here: Policy.MaxAttempts is the
// configured retry count plus the first run, because guard.Attempt requires
// 1 <= Current <= Max.
package failure
