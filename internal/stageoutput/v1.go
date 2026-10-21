package stageoutput

import (
	"bytes"
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

// wireV1 is the schema_version 1 wire form. The yaml tags here define the
// closed key set for version 1 and, because yaml.v3 emits struct fields in
// declaration order, the canonical key order on output.
//
// Body and Unresolved are pointers so presence can be told apart from an empty
// value: a punch_out document with body: "" or a canonical document with
// unresolved: [] must be rejected as carrying a forbidden field. An explicit
// YAML null decodes to a nil pointer and is treated as absent.
type wireV1 struct {
	SchemaVersion int               `yaml:"schema_version"`
	Stage         string            `yaml:"stage"`
	Result        string            `yaml:"result"`
	Source        wireSource        `yaml:"source"`
	Generated     string            `yaml:"generated"`
	Body          *string           `yaml:"body,omitempty"`
	Unresolved    *[]wireUnresolved `yaml:"unresolved,omitempty"`
	Inferences    []wireInference   `yaml:"inferences,omitempty"`
}

type wireSource struct {
	Kind string `yaml:"kind"`
	Ref  string `yaml:"ref"`
}

type wireUnresolved struct {
	Slot   string `yaml:"slot"`
	Reason string `yaml:"reason"`
}

type wireInference struct {
	Slot     string `yaml:"slot"`
	Inferred string `yaml:"inferred"`
	Evidence string `yaml:"evidence"`
}

// bodyKey is the YAML key whose value is written as a literal block scalar.
const bodyKey = "body"

// wireV1FromDocument converts a Document to its version 1 wire form. A
// non-empty Body or Unresolved is emitted; an empty one is omitted.
func wireV1FromDocument(d *Document) *wireV1 {
	w := &wireV1{
		SchemaVersion: d.SchemaVersion,
		Stage:         string(d.Stage),
		Result:        string(d.Result),
		Source:        wireSource{Kind: string(d.Source.Kind), Ref: d.Source.Ref},
		Generated:     d.Generated,
	}
	if d.Body != "" {
		body := d.Body
		w.Body = &body
	}
	if len(d.Unresolved) > 0 {
		list := make([]wireUnresolved, len(d.Unresolved))
		for i, u := range d.Unresolved {
			list[i] = wireUnresolved{Slot: u.Slot, Reason: u.Reason}
		}
		w.Unresolved = &list
	}
	for _, inf := range d.Inferences {
		w.Inferences = append(w.Inferences, wireInference{Slot: inf.Slot, Inferred: inf.Inferred, Evidence: inf.Evidence})
	}
	return w
}

// node encodes w to a yaml.Node mapping and marks the body value as a literal
// block scalar. Going through a Node keeps the struct tags as the single
// source of key names and order.
func (w *wireV1) node() (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(w); err != nil {
		return nil, err
	}
	root := &n
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == bodyKey {
			root.Content[i+1].Style = yaml.LiteralStyle
		}
	}
	return root, nil
}

// parseV1 strictly decodes a schema_version 1 document, converts it to a
// Document and validates it.
func parseV1(data []byte) (*Document, error) {
	// Use a decoder with KnownFields(true) so unknown or misspelled keys, at
	// the top level and inside source, inferences[] and unresolved[], are
	// rejected rather than silently dropped. This is the same pattern as
	// internal/plan/parser.go: a dropped key would otherwise corrupt the
	// document without anyone noticing.
	var w wireV1
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&w); err != nil {
		return nil, decodeError(err)
	}

	// A second Decode must hit end of input. Anything else is a second
	// document (or garbage after the first), which a single stage output must
	// not contain.
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, invalidf("input must contain exactly one YAML document")
	}

	d := w.document()
	var problems []string
	if err := d.Validate(); err != nil {
		var verr *ValidationError
		if errors.As(err, &verr) {
			problems = append(problems, verr.Problems...)
		} else {
			return nil, err
		}
	}
	problems = append(problems, w.presenceProblems()...)
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return d, nil
}

// document converts the wire form to a Document. A nil Body or Unresolved
// pointer (absent, or explicit null) becomes the zero value.
func (w *wireV1) document() *Document {
	d := &Document{
		SchemaVersion: w.SchemaVersion,
		Stage:         Stage(w.Stage),
		Result:        Result(w.Result),
		Source:        Source{Kind: SourceKind(w.Source.Kind), Ref: w.Source.Ref},
		Generated:     w.Generated,
	}
	if w.Body != nil {
		d.Body = *w.Body
	}
	if w.Unresolved != nil {
		for _, u := range *w.Unresolved {
			d.Unresolved = append(d.Unresolved, Unresolved{Slot: u.Slot, Reason: u.Reason})
		}
	}
	for _, inf := range w.Inferences {
		d.Inferences = append(d.Inferences, Inference{Slot: inf.Slot, Inferred: inf.Inferred, Evidence: inf.Evidence})
	}
	return d
}

// presenceProblems reports variant-specific keys that are present but empty.
// Document cannot tell body: "" from an absent body, or unresolved: [] from an
// absent list, so Validate lets them through; the wire form can tell, and the
// union rules are about presence, not emptiness.
func (w *wireV1) presenceProblems() []string {
	var problems []string
	switch Result(w.Result) {
	case ResultPunchOut:
		if w.Body != nil && *w.Body == "" {
			problems = append(problems, "body: not allowed when result is punch_out (present but empty)")
		}
	case ResultCanonical:
		if w.Unresolved != nil && len(*w.Unresolved) == 0 {
			problems = append(problems, "unresolved: not allowed when result is canonical (present but empty)")
		}
	}
	return problems
}
