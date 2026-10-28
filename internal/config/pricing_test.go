package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

var wantSonnetIDs = []string{
	"claude-sonnet-5.5",
	"claude-sonnet-5",
	"claude-sonnet-4.6",
	"claude-sonnet-4.5",
	"claude-sonnet-4",
}

func TestDefaultPricing_SonnetAt3And15NoHaiku(t *testing.T) {
	got := DefaultPricing()
	if len(got) != len(wantSonnetIDs) {
		t.Errorf("len(DefaultPricing()) = %d, want %d: %v", len(got), len(wantSonnetIDs), got)
	}
	for _, id := range wantSonnetIDs {
		p, ok := got[id]
		if !ok {
			t.Errorf("DefaultPricing() missing %q", id)
			continue
		}
		if p.InputUSDPerMTok != 3 || p.OutputUSDPerMTok != 15 {
			t.Errorf("DefaultPricing()[%q] = %+v, want input 3 output 15", id, p)
		}
	}
	if _, ok := got["claude-haiku-4.5"]; ok {
		t.Error("DefaultPricing() must not price claude-haiku-4.5")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("DefaultPricing().Validate() = %v, want nil", err)
	}
}

func TestDefaultPricing_ReturnsFreshTable(t *testing.T) {
	a := DefaultPricing()
	a["claude-sonnet-5.5"] = inference.Price{InputUSDPerMTok: 99, OutputUSDPerMTok: 99}
	delete(a, "claude-sonnet-4")
	b := DefaultPricing()
	if p := b["claude-sonnet-5.5"]; p.InputUSDPerMTok != 3 || p.OutputUSDPerMTok != 15 {
		t.Errorf("mutation leaked into later defaults: %+v", p)
	}
	if _, ok := b["claude-sonnet-4"]; !ok {
		t.Error("delete leaked into later defaults")
	}
}

func TestLoadPricing_PresentKeyReplacesDefaults(t *testing.T) {
	// One check per row: the empty mapping keeps its len == 0 assertion.
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, got inference.PriceTable)
	}{
		{
			name: "one_entry",
			yaml: `pricing:
  my-model:
    input_usd_per_mtok: 1.5
    output_usd_per_mtok: 7
`,
			check: func(t *testing.T, got inference.PriceTable) {
				want := inference.PriceTable{"my-model": {InputUSDPerMTok: 1.5, OutputUSDPerMTok: 7}}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("LoadPricing() = %v, want exactly %v (no merge with defaults)", got, want)
				}
			},
		},
		{
			name: "empty_mapping",
			yaml: "pricing: {}\n",
			check: func(t *testing.T, got inference.PriceTable) {
				if len(got) != 0 {
					t.Errorf("LoadPricing() = %v, want empty table", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tt.yaml))
			got, err := LoadPricing()
			if err != nil {
				t.Fatalf("LoadPricing() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestLoadPricing_MalformedYAMLErrors(t *testing.T) {
	writeProjectConfig(t, strPtr("pricing: [unterminated\n"))
	if _, err := LoadPricing(); err == nil {
		t.Fatal("LoadPricing() error = nil, want parse error")
	}
}

func TestLoadPricing_ReadErrorOtherThanNotExist(t *testing.T) {
	writeProjectConfig(t, nil)
	// A directory where the file should be: ReadFile fails with a non-ENOENT error.
	if err := os.MkdirAll(".kairon/config.yaml", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := LoadPricing(); err == nil {
		t.Fatal("LoadPricing() error = nil, want read error")
	}
}

func TestLoadPricing_NotAMappingErrors(t *testing.T) {
	writeProjectConfig(t, strPtr("pricing:\n  - claude-sonnet-5.5\n"))
	if _, err := LoadPricing(); err == nil {
		t.Fatal("LoadPricing() error = nil, want error for non-mapping pricing")
	}
}

var invalidPricingCases = []struct {
	name    string
	yaml    string
	wantErr []string // substrings the error must contain
}{
	{
		name: "missing output field",
		yaml: `pricing:
  model-a:
    input_usd_per_mtok: 3
`,
		wantErr: []string{"model-a", "output_usd_per_mtok"},
	},
	{
		name: "missing input field",
		yaml: `pricing:
  model-b:
    output_usd_per_mtok: 3
`,
		wantErr: []string{"model-b", "input_usd_per_mtok"},
	},
	{
		name: "negative input price",
		yaml: `pricing:
  model-c:
    input_usd_per_mtok: -1
    output_usd_per_mtok: 15
`,
		wantErr: []string{"model-c", "negative"},
	},
	{
		name: "negative output price",
		yaml: `pricing:
  model-d:
    input_usd_per_mtok: 3
    output_usd_per_mtok: -0.5
`,
		wantErr: []string{"model-d", "negative"},
	},
	{
		name: "empty key",
		yaml: `pricing:
  "":
    input_usd_per_mtok: 3
    output_usd_per_mtok: 15
`,
		wantErr: []string{"empty"},
	},
	{
		name: "whitespace padded key",
		yaml: `pricing:
  " padded-model ":
    input_usd_per_mtok: 3
    output_usd_per_mtok: 15
`,
		wantErr: []string{" padded-model "},
	},
	{
		name: "entry is not a mapping",
		yaml: `pricing:
  model-e: 3
`,
		wantErr: []string{"model-e"},
	},
	{
		name: "null entry",
		yaml: `pricing:
  model-f: null
`,
		wantErr: []string{"model-f", "null"},
	},
}

func TestLoadPricing_InvalidEntryRejectedNamingModel(t *testing.T) {
	for _, tc := range invalidPricingCases {
		t.Run(tc.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tc.yaml))
			_, err := LoadPricing()
			if err == nil {
				t.Fatal("LoadPricing() error = nil, want validation error")
			}
			for _, sub := range tc.wantErr {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error %q does not contain %q", err, sub)
				}
			}
		})
	}
}

func TestLoad_InvalidPricingRejectedNamingModel(t *testing.T) {
	for _, tc := range invalidPricingCases {
		t.Run(tc.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr("repo: owner/name\n"+tc.yaml))
			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), "invalid pricing config") {
				t.Errorf("error %q does not mention invalid pricing config", err)
			}
			for _, sub := range tc.wantErr {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error %q does not contain %q", err, sub)
				}
			}
		})
	}
}
