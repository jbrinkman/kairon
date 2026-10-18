package watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvenance_String(t *testing.T) {
	tests := []struct {
		p    Provenance
		want string
	}{
		{ProvenanceValid, "valid"},
		{ProvenanceNone, "none"},
		{ProvenanceMismatch, "mismatch"},
		{ProvenanceUnknown, "unknown"},
		{Provenance(99), "unknown"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.p.String())
	}
}

func TestProvenance_ZeroValueIsUnknown(t *testing.T) {
	var p Provenance
	assert.Equal(t, ProvenanceUnknown, p)
	assert.NotEqual(t, ProvenanceValid, p)
}

func TestProvenanceClassifierFunc_DelegatesToFunction(t *testing.T) {
	var got string
	var c ProvenanceClassifier = ProvenanceClassifierFunc(func(body string) Provenance {
		got = body
		return ProvenanceMismatch
	})

	assert.Equal(t, ProvenanceMismatch, c.Classify("the body"))
	assert.Equal(t, "the body", got)
}
