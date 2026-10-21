package stageoutput

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// versionParsers maps each supported schema_version to the strict parser for
// that version. Adding a key to the document means adding a new wire struct
// and registering its parser here under the next version number, then raising
// CurrentSchemaVersion. A version that is not in this table is rejected with
// an UnknownVersionError before any strict decode happens. Each registered
// parser decodes with yaml.v3 and KnownFields(true) (see parseV1 in v1.go).
var versionParsers = map[int]func([]byte) (*Document, error){
	1: parseV1,
}

// versionProbe reads only the schema_version key. It is decoded without
// KnownFields on purpose: a document from a newer schema legitimately carries
// keys this parser does not know, and those must surface as an unknown version
// rather than as an unknown field. The value stays a yaml.Node so its tag can
// be checked; decoding straight into an int would let yaml.v3 silently truncate
// 1.5 to 1.
type versionProbe struct {
	SchemaVersion yaml.Node `yaml:"schema_version"`
}

// Parse reads one stage-output document from data and validates it.
//
// Parsing is version-scoped. The schema_version is read first with a lenient
// probe; a version this package does not support returns an
// *UnknownVersionError (errors.Is(err, ErrUnknownVersion)) before any strict
// decode. A supported version is then decoded strictly: unknown or misspelled
// keys at any depth are errors. Every malformed-document failure, including
// syntax errors, empty input, non-mapping input and multi-document streams,
// matches ErrInvalidDocument.
//
// JSON is valid YAML, so a JSON-shaped payload flows through the same path.
func Parse(data []byte) (*Document, error) {
	version, err := probeVersion(data)
	if err != nil {
		return nil, err
	}
	parse, ok := versionParsers[version]
	if !ok {
		return nil, &UnknownVersionError{Version: version, MaxSupported: CurrentSchemaVersion}
	}
	return parse(data)
}

// probeVersion returns the declared schema_version, or an error if the input
// is empty, is not a mapping, or does not declare an integer version >= 1.
func probeVersion(data []byte) (int, error) {
	var probe versionProbe
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&probe); err != nil {
		return 0, decodeError(err)
	}

	node := probe.SchemaVersion
	if node.Kind == 0 {
		return 0, invalidf("schema_version: required")
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		return 0, invalidf("schema_version: must be an integer >= 1, got %s", describeNode(&node))
	}
	var version int
	if err := node.Decode(&version); err != nil {
		return 0, invalidf("schema_version: must be an integer >= 1, got %s", describeNode(&node))
	}
	if version < 1 {
		return 0, invalidf("schema_version: must be an integer >= 1, got %d", version)
	}
	return version, nil
}

// describeNode renders a node's value for an error message.
func describeNode(n *yaml.Node) string {
	if n.Kind != yaml.ScalarNode {
		return "a non-scalar value"
	}
	return fmt.Sprintf("%q (%s)", n.Value, n.Tag)
}

// invalidf returns an error that matches ErrInvalidDocument.
func invalidf(format string, args ...any) error {
	return &ValidationError{Problems: []string{fmt.Sprintf(format, args...)}}
}

// decodeError converts a yaml.v3 decode failure into an ErrInvalidDocument
// error. Empty input surfaces as io.EOF. Syntax errors, type mismatches
// (including a non-mapping document) and unknown keys all land here.
func decodeError(err error) error {
	if errors.Is(err, io.EOF) {
		return invalidf("document is empty")
	}
	return fmt.Errorf("%w: failed to parse stage-output YAML: %w", ErrInvalidDocument, err)
}
