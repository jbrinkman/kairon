// Package provenance canonicalizes, hashes, marks and classifies GitHub issue
// bodies so the pipeline can tell whether a body was written by the
// deterministic layer after a guard chain passed.
//
// The marker is a routing and classification signal. It is not a control that
// makes the content trustworthy: anyone who can edit the issue can recompute a
// matching hash. The package therefore carries no HMAC, key or secret.
//
// The package is pure. Non-test code may import only the standard library,
// internal/guard and internal/stageoutput, and must not import os, os/exec,
// net, net/http or crypto/hmac. All GitHub I/O goes through an interface so
// every behaviour can be tested with a fake. TestNoForbiddenImports enforces
// the rule.
package provenance
