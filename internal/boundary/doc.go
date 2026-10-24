// Package boundary is the consuming-side load-and-verify step between the
// watcher and the implementation pipeline.
//
// Admit takes only a repository and an issue number. It reads the issue from
// GitHub (the transport), classifies the body it read with internal/provenance,
// and either admits it as an InputDocument, skips it, or tells the caller to
// punch it out. A valid hash is a routing signal only: the body stays untrusted
// data and is screened on every path.
//
// The package is pure. Non-test code may import only the standard library,
// gopkg.in/yaml.v3, internal/provenance, internal/failure, internal/guard and
// internal/stageoutput, and must not import os, os/exec, net, net/http,
// internal/github, internal/agent or internal/watcher. GitHub reads go through
// the Reader port and intake routing through the Intake port; the package never
// writes to GitHub. TestNoForbiddenImports enforces the rule.
package boundary
