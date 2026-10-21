// Package stageoutput defines the document a pipeline stage returns: a YAML
// mapping with a metadata envelope (schema_version, stage, result, source,
// generated) and a body, parsed under a strict, version-scoped schema.
//
// The result field is a discriminated union. A "canonical" document carries a
// body; a "punch_out" document carries a non-empty unresolved list instead and
// no body. Validate enforces this and the envelope rules, and Marshal refuses
// to emit a document that fails Validate.
//
// This package imports only the standard library and gopkg.in/yaml.v3. It must
// not import any other internal package, so that stages and guards can depend
// on it without creating an import cycle (enforced by TestNoInternalDeps).
//
// The document format is described in docs/stage-output.md.
package stageoutput
