package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
	"gopkg.in/yaml.v3"
)

// shippedConfigFiles are the two config.yaml files that must carry an active,
// documented pricing block: the project's own and the template that
// `kairon init` writes for new projects.
var shippedConfigFiles = map[string]string{
	"project":  filepath.Join("..", "..", ".kairon", "config.yaml"),
	"template": filepath.Join("..", "..", "cmd", "kairon", "templates", "kairon", "config.yaml"),
}

// sonnetModels returns the Sonnet ids in DefaultEvalsConfig().AllowedModels.
func sonnetModels(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, m := range DefaultEvalsConfig().AllowedModels {
		if strings.Contains(m, "sonnet") {
			ids = append(ids, m)
		}
	}
	if len(ids) == 0 {
		t.Fatal("DefaultEvalsConfig().AllowedModels has no Sonnet model")
	}
	return ids
}

// readShipped returns the file's bytes, failing the test when it is missing.
func readShipped(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// TestShippedConfigFiles_PricingBlockPricesEverySonnetModel parses the raw
// YAML of both files, so it fails when the pricing block is absent (the
// built-in defaults would otherwise satisfy a LoadPricing-based assertion).
func TestShippedConfigFiles_PricingBlockPricesEverySonnetModel(t *testing.T) {
	for name, path := range shippedConfigFiles {
		t.Run(name, func(t *testing.T) {
			data := readShipped(t, path)

			var raw struct {
				Pricing map[string]inference.Price `yaml:"pricing"`
			}
			if err := yaml.Unmarshal(data, &raw); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if raw.Pricing == nil {
				t.Fatalf("%s has no active pricing: block", path)
			}
			for _, id := range sonnetModels(t) {
				p, ok := raw.Pricing[id]
				if !ok {
					t.Errorf("%s: pricing has no entry for %q", path, id)
					continue
				}
				if p.InputUSDPerMTok != 3 {
					t.Errorf("%s: pricing[%q].input_usd_per_mtok = %v, want 3", path, id, p.InputUSDPerMTok)
				}
				if p.OutputUSDPerMTok != 15 {
					t.Errorf("%s: pricing[%q].output_usd_per_mtok = %v, want 15", path, id, p.OutputUSDPerMTok)
				}
			}
			// Haiku is allowed to run but is intentionally unpriced.
			if _, ok := raw.Pricing["claude-haiku-4.5"]; ok {
				t.Errorf("%s: claude-haiku-4.5 must stay unpriced", path)
			}
		})
	}
}

// TestShippedConfigFiles_PricingBlockIsDocumented checks that the block is
// visible and explained, not just present.
func TestShippedConfigFiles_PricingBlockIsDocumented(t *testing.T) {
	for name, path := range shippedConfigFiles {
		t.Run(name, func(t *testing.T) {
			text := string(readShipped(t, path))
			if !strings.Contains(text, "\npricing:\n") {
				t.Fatalf("%s: no uncommented top-level `pricing:` line", path)
			}
			lower := strings.ToLower(text)
			for _, want := range []string{
				"exact",            // keys are exact model ids
				"per million",      // unit
				"unknown",          // unpriced model = unknown cost
				"replace",          // replace, not merge
				"claude-haiku-4.5", // intentionally unpriced
			} {
				if !strings.Contains(lower, want) {
					t.Errorf("%s: pricing comments do not mention %q", path, want)
				}
			}
		})
	}
}

// TestShippedConfigFiles_Loadable runs the real loaders against both files.
func TestShippedConfigFiles_Loadable(t *testing.T) {
	for name, path := range shippedConfigFiles {
		t.Run(name, func(t *testing.T) {
			content := string(readShipped(t, path))
			// The template ships `repo: ""` for the user to fill in; Load
			// rightly requires it, so give it a value for this check only.
			content = strings.Replace(content, `repo: ""`, `repo: owner/name`, 1)
			writeProjectConfig(t, &content)

			prices, err := LoadPricing()
			if err != nil {
				t.Fatalf("LoadPricing() error = %v", err)
			}
			for _, id := range sonnetModels(t) {
				if p := prices[id]; p.InputUSDPerMTok != 3 || p.OutputUSDPerMTok != 15 {
					t.Errorf("LoadPricing()[%q] = %+v, want input 3 output 15", id, p)
				}
			}
			if _, ok := prices["claude-haiku-4.5"]; ok {
				t.Error("LoadPricing() must not price claude-haiku-4.5")
			}

			if _, err := LoadEvals(); err != nil {
				t.Errorf("LoadEvals() error = %v", err)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(cfg.Pricing) != len(prices) {
				t.Errorf("Load().Pricing has %d entries, LoadPricing() has %d", len(cfg.Pricing), len(prices))
			}
		})
	}
}
