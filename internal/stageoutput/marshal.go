package stageoutput

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Marshal validates d and serializes it to YAML. A document that fails
// Validate, or that declares a schema_version this package cannot write,
// returns an error and no bytes.
//
// Keys are emitted in the canonical order: schema_version, stage, result,
// source, generated, body or unresolved, inferences. The body is written as a
// literal block scalar. yaml.v3 falls back to a quoted scalar when a literal
// block cannot represent the text (for example a line with trailing spaces);
// the guarantee is that the body decodes back to the identical string, not that
// the style is always a block scalar.
func Marshal(d *Document) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	// Marshal checks CurrentSchemaVersion, not the versionParsers registry: the
	// registry lists versions Parse can read, while Marshal can only write the
	// wire form it has an encoder for. A new version adds both together.
	if d.SchemaVersion != CurrentSchemaVersion {
		return nil, &UnknownVersionError{Version: d.SchemaVersion, MaxSupported: CurrentSchemaVersion}
	}

	node, err := wireV1FromDocument(d).node()
	if err != nil {
		return nil, fmt.Errorf("failed to encode stage-output document: %w", err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("failed to encode stage-output document: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to encode stage-output document: %w", err)
	}
	return buf.Bytes(), nil
}
