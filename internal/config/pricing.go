package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"

	"github.com/jbrinkman/kairon/internal/inference"
	"gopkg.in/yaml.v3"
)

// DefaultPricing returns the built-in price table: the Sonnet model ids in
// DefaultEvalsConfig().AllowedModels at $3 input / $15 output per million
// tokens. claude-haiku-4.5 is deliberately absent: it may run but has no
// price, so its cost is reported as unknown. The returned table is freshly
// allocated on every call, so callers may mutate it.
func DefaultPricing() inference.PriceTable {
	sonnet := inference.Price{InputUSDPerMTok: 3, OutputUSDPerMTok: 15}
	return inference.PriceTable{
		"claude-sonnet-5.5": sonnet,
		"claude-sonnet-5":   sonnet,
		"claude-sonnet-4.6": sonnet,
		"claude-sonnet-4.5": sonnet,
		"claude-sonnet-4":   sonnet,
	}
}

// pricingFile selects only the top-level `pricing` key of config.yaml. It is
// kept as a raw node so an absent key can be told apart from a present one and
// so each entry can be decoded (and any error attributed) per model id.
type pricingFile struct {
	Pricing yaml.Node `yaml:"pricing"`
}

// pricingFromYAML builds the effective price table from config.yaml bytes.
// An absent (or null) `pricing` key yields DefaultPricing(). A present key
// yields exactly its entries: it replaces the defaults rather than merging
// with them, so an operator can remove a price to make a model's cost unknown.
// The table is validated; every error names the offending model id.
func pricingFromYAML(data []byte) (inference.PriceTable, error) {
	var file pricingFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	node := &file.Pricing
	if node.Kind == 0 || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		return DefaultPricing(), nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("invalid pricing config: pricing must be a mapping of model id to price")
	}

	var entries map[string]yaml.Node
	if err := node.Decode(&entries); err != nil {
		return nil, fmt.Errorf("invalid pricing config: %w", err)
	}

	// Sorted so the reported error is deterministic.
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	table := make(inference.PriceTable, len(entries))
	for _, id := range ids {
		entry := entries[id]
		// A null entry (`model: null`) bypasses Price.UnmarshalYAML and would
		// decode to a zero Price, which Validate accepts -- making the model
		// look like a known $0 price instead of an unknown cost. Reject it so
		// a free model must be written explicitly as a pair of zero rates.
		if entry.Kind == yaml.ScalarNode && entry.Tag == "!!null" {
			return nil, fmt.Errorf("invalid pricing config: model %q: price must not be null; "+
				"set input_usd_per_mtok and output_usd_per_mtok (both 0 for a free model)", id)
		}
		var p inference.Price
		if err := entry.Decode(&p); err != nil {
			return nil, fmt.Errorf("invalid pricing config: model %q: %w", id, err)
		}
		table[id] = p
	}
	if err := table.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pricing config: %w", err)
	}
	return table, nil
}

// LoadPricing reads only the `pricing` key of ./.kairon/config.yaml. A missing
// file or an absent key yields the defaults; any other read error, a YAML
// parse error or an invalid entry is returned. Unlike Load it does not
// require `repo` and does not load a theme.
func LoadPricing() (inference.PriceTable, error) {
	const path = ".kairon/config.yaml"

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultPricing(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return pricingFromYAML(data)
}
