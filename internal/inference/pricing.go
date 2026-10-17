package inference

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Price is a model's price in USD per million tokens.
type Price struct {
	InputUSDPerMTok  float64 `yaml:"input_usd_per_mtok"  json:"input_usd_per_mtok"`
	OutputUSDPerMTok float64 `yaml:"output_usd_per_mtok" json:"output_usd_per_mtok"`
}

// PriceTable maps an EXACT model id (no trimming, no case folding) to its
// Price. A model with no entry has an unknown cost; there is no fallback to
// another model's price.
type PriceTable map[string]Price

// Cost is the priced result of one call's usage. When Known is false, USD is 0
// because the model has no configured price, not because the call was free.
type Cost struct {
	USD   float64
	Known bool
}

// Cost prices u under model. A call that used no tokens spent nothing, so it
// is Known with USD 0 whatever the model. Otherwise the model must be listed
// (exact id) for the cost to be Known; an unlisted model, including the empty
// model, yields {USD: 0, Known: false}. A nil or empty table prices nothing.
func (t PriceTable) Cost(model string, u Usage) Cost {
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return Cost{USD: 0, Known: true}
	}
	p, ok := t[model]
	if !ok {
		return Cost{USD: 0, Known: false}
	}
	usd := float64(u.InputTokens)*p.InputUSDPerMTok/1_000_000 +
		float64(u.OutputTokens)*p.OutputUSDPerMTok/1_000_000
	return Cost{USD: usd, Known: true}
}

// Validate rejects empty and whitespace-padded model ids, and prices that are
// negative, NaN or infinite (a table built in code bypasses UnmarshalYAML).
// Errors name the offending model id. Keys are checked in sorted order so the
// reported error is deterministic.
func (t PriceTable) Validate() error {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "" {
			return errors.New("pricing: model id must not be empty")
		}
		if strings.TrimSpace(k) != k {
			return fmt.Errorf("pricing: model id %q has leading or trailing whitespace", k)
		}
		if err := t[k].validate(); err != nil {
			return fmt.Errorf("pricing: model %q: %w", k, err)
		}
	}
	return nil
}

func (p Price) validate() error {
	for _, f := range []struct {
		name string
		v    float64
	}{
		{"input_usd_per_mtok", p.InputUSDPerMTok},
		{"output_usd_per_mtok", p.OutputUSDPerMTok},
	} {
		if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
			return fmt.Errorf("%s must be a finite number", f.name)
		}
		if f.v < 0 {
			return fmt.Errorf("%s must not be negative", f.name)
		}
	}
	return nil
}

// UnmarshalYAML decodes a Price, requiring both fields and rejecting negative,
// NaN and infinite values. Requiring both stops a typo or omission from
// silently pricing one side of a call at zero.
func (p *Price) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Input  *float64 `yaml:"input_usd_per_mtok"`
		Output *float64 `yaml:"output_usd_per_mtok"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Input == nil || raw.Output == nil {
		return errors.New("pricing entry needs input_usd_per_mtok and output_usd_per_mtok")
	}
	candidate := Price{InputUSDPerMTok: *raw.Input, OutputUSDPerMTok: *raw.Output}
	if err := candidate.validate(); err != nil {
		return err
	}
	*p = candidate
	return nil
}
