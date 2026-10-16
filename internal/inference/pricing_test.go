package inference

import (
	"math"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testTable() PriceTable {
	return PriceTable{
		"claude-sonnet-5.5": {InputUSDPerMTok: 3, OutputUSDPerMTok: 15},
		"cheap":             {InputUSDPerMTok: 0.8, OutputUSDPerMTok: 4},
	}
}

func TestPriceTable_Cost_Known(t *testing.T) {
	got := testTable().Cost("claude-sonnet-5.5", Usage{InputTokens: 1000, OutputTokens: 500})
	want := float64(1000)*3/1_000_000 + float64(500)*15/1_000_000
	if !got.Known || got.USD != want {
		t.Errorf("Cost = %+v, want {USD:%v Known:true}", got, want)
	}
	if math.Abs(got.USD-0.0105) > 1e-12 {
		t.Errorf("USD = %v, want ~0.0105", got.USD)
	}
}

func TestPriceTable_Cost_UsesPerModelPrice(t *testing.T) {
	u := Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	if got := testTable().Cost("cheap", u); !got.Known || got.USD != 4.8 {
		t.Errorf("cheap Cost = %+v, want 4.8 known", got)
	}
}

func TestPriceTable_Cost_UnknownModel(t *testing.T) {
	u := Usage{InputTokens: 10, OutputTokens: 10}
	for _, model := range []string{"claude-haiku-4.5", "", "Claude-Sonnet-5.5", " claude-sonnet-5.5", "claude-sonnet-5.5 "} {
		got := testTable().Cost(model, u)
		if got.Known || got.USD != 0 {
			t.Errorf("Cost(%q) = %+v, want {0 false} (exact-id lookup only)", model, got)
		}
	}
}

func TestPriceTable_Cost_NilAndEmptyTable(t *testing.T) {
	u := Usage{InputTokens: 1, OutputTokens: 1}
	for name, tbl := range map[string]PriceTable{"nil": nil, "empty": {}} {
		if got := tbl.Cost("claude-sonnet-5.5", u); got.Known || got.USD != 0 {
			t.Errorf("%s table: Cost = %+v, want {0 false}", name, got)
		}
	}
}

func TestPriceTable_Cost_NoTokensIsKnownZero(t *testing.T) {
	for _, model := range []string{"claude-sonnet-5.5", "unlisted", ""} {
		for name, tbl := range map[string]PriceTable{"table": testTable(), "nil": nil} {
			got := tbl.Cost(model, Usage{})
			if !got.Known || got.USD != 0 {
				t.Errorf("%s Cost(%q, 0 tokens) = %+v, want {0 true}", name, model, got)
			}
		}
	}
}

func TestPriceTable_Cost_OneSidedTokensNeedPrice(t *testing.T) {
	if got := testTable().Cost("unlisted", Usage{OutputTokens: 1}); got.Known {
		t.Errorf("Cost = %+v, want unknown", got)
	}
	if got := testTable().Cost("claude-sonnet-5.5", Usage{InputTokens: 1_000_000}); !got.Known || got.USD != 3 {
		t.Errorf("Cost = %+v, want {3 true}", got)
	}
}

func TestPrice_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		want    Price
		wantErr string
	}{
		{"ok", "input_usd_per_mtok: 3\noutput_usd_per_mtok: 15\n", Price{3, 15}, ""},
		{"zero is allowed", "input_usd_per_mtok: 0\noutput_usd_per_mtok: 0\n", Price{0, 0}, ""},
		{"fractional", "input_usd_per_mtok: 0.25\noutput_usd_per_mtok: 1.25\n", Price{0.25, 1.25}, ""},
		{"missing input", "output_usd_per_mtok: 15\n", Price{}, "input_usd_per_mtok and output_usd_per_mtok"},
		{"missing output", "input_usd_per_mtok: 3\n", Price{}, "input_usd_per_mtok and output_usd_per_mtok"},
		{"empty mapping", "{}\n", Price{}, "input_usd_per_mtok and output_usd_per_mtok"},
		{"negative input", "input_usd_per_mtok: -1\noutput_usd_per_mtok: 15\n", Price{}, "negative"},
		{"negative output", "input_usd_per_mtok: 3\noutput_usd_per_mtok: -0.5\n", Price{}, "negative"},
		{"nan", "input_usd_per_mtok: .nan\noutput_usd_per_mtok: 15\n", Price{}, "finite"},
		{"inf", "input_usd_per_mtok: 3\noutput_usd_per_mtok: .inf\n", Price{}, "finite"},
		{"neg inf", "input_usd_per_mtok: -.inf\noutput_usd_per_mtok: 15\n", Price{}, "finite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Price
			err := yaml.Unmarshal([]byte(tt.doc), &p)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p != tt.want {
				t.Errorf("Price = %+v, want %+v", p, tt.want)
			}
		})
	}
}

func TestPriceTable_UnmarshalYAML_Map(t *testing.T) {
	doc := "m1:\n  input_usd_per_mtok: 3\n  output_usd_per_mtok: 15\nm2:\n  input_usd_per_mtok: 1\n  output_usd_per_mtok: 2\n"
	var tbl PriceTable
	if err := yaml.Unmarshal([]byte(doc), &tbl); err != nil {
		t.Fatal(err)
	}
	if len(tbl) != 2 || tbl["m1"] != (Price{3, 15}) || tbl["m2"] != (Price{1, 2}) {
		t.Errorf("table = %+v", tbl)
	}
	bad := "m1:\n  input_usd_per_mtok: 3\n"
	if err := yaml.Unmarshal([]byte(bad), &tbl); err == nil {
		t.Error("expected error for entry missing output price")
	}
}

func TestPriceTable_Validate(t *testing.T) {
	if err := testTable().Validate(); err != nil {
		t.Errorf("valid table: %v", err)
	}
	if err := PriceTable(nil).Validate(); err != nil {
		t.Errorf("nil table: %v", err)
	}
	for _, key := range []string{"", " ", " m", "m ", "\tm", "m\n"} {
		err := PriceTable{key: {1, 1}}.Validate()
		if err == nil {
			t.Errorf("key %q: expected error", key)
		}
	}
	err := PriceTable{" padded ": {1, 1}}.Validate()
	if err == nil || !strings.Contains(err.Error(), " padded ") {
		t.Errorf("error should name the key, got %v", err)
	}
}

func TestPriceTable_Validate_RejectsBadPrices(t *testing.T) {
	// Tables built in code (not via YAML) are validated too.
	for name, p := range map[string]Price{
		"negative": {-1, 1},
		"nan":      {math.NaN(), 1},
		"inf":      {1, math.Inf(1)},
	} {
		err := PriceTable{"m": p}.Validate()
		if err == nil || !strings.Contains(err.Error(), "m") {
			t.Errorf("%s: err = %v, want error naming m", name, err)
		}
	}
}
