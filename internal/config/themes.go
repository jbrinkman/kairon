package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Theme struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Colors      struct {
		Primary       string `yaml:"primary"`
		Secondary     string `yaml:"secondary"`
		Success       string `yaml:"success"`
		Warning       string `yaml:"warning"`
		Error         string `yaml:"error"`
		TextPrimary   string `yaml:"text_primary"`
		TextSecondary string `yaml:"text_secondary"`
		TextMuted     string `yaml:"text_muted"`
		Prompt        string `yaml:"prompt"`
		Separator     string `yaml:"separator"`
		Activity      string `yaml:"activity"`
		Background    string `yaml:"background"`
		Surface       string `yaml:"surface"`
		AgentSuccess  string `yaml:"agent_success"`
		AgentFail     string `yaml:"agent_fail"`
	} `yaml:"colors"`
}

var validColorPattern = regexp.MustCompile(`^(#[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{3}|\d{1,3}|[a-zA-Z-]+)$`)
var validThemeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var namedColors = map[string]bool{
	"black": true, "red": true, "green": true, "yellow": true,
	"blue": true, "magenta": true, "cyan": true, "white": true,
	"gray": true, "grey": true, "bright-black": true, "bright-red": true,
	"bright-green": true, "bright-yellow": true, "bright-blue": true,
	"bright-magenta": true, "bright-cyan": true, "bright-white": true,
}

func validateColor(color, field string) error {
	if color == "" {
		return fmt.Errorf("color field '%s' is required but empty", field)
	}

	if !validColorPattern.MatchString(color) {
		return fmt.Errorf("invalid color format for '%s': %s (expected hex #RRGGBB, #RGB, ANSI code 0-255, or named color)", field, color)
	}

	// Validate ANSI codes (0-255)
	if regexp.MustCompile(`^\d+$`).MatchString(color) {
		if len(color) > 3 {
			return fmt.Errorf("invalid color format for '%s': %s (ANSI codes must be 0-255)", field, color)
		}
		// Parse and check range
		if code := 0; len(color) > 0 {
			for _, r := range color {
				code = code*10 + int(r-'0')
			}
			if code > 255 {
				return fmt.Errorf("invalid color format for '%s': %s (ANSI codes must be 0-255)", field, color)
			}
		}
		return nil
	}

	// Validate named colors
	if !strings.HasPrefix(color, "#") {
		if !namedColors[strings.ToLower(color)] {
			return fmt.Errorf("unknown named color for '%s': %s", field, color)
		}
	}

	return nil
}

func validateTheme(theme *Theme) error {
	if theme.Name == "" {
		return fmt.Errorf("theme name is required")
	}

	// Validate all required color fields
	colorFields := map[string]string{
		"primary":        theme.Colors.Primary,
		"secondary":      theme.Colors.Secondary,
		"success":        theme.Colors.Success,
		"warning":        theme.Colors.Warning,
		"error":          theme.Colors.Error,
		"text_primary":   theme.Colors.TextPrimary,
		"text_secondary": theme.Colors.TextSecondary,
		"text_muted":     theme.Colors.TextMuted,
		"prompt":         theme.Colors.Prompt,
		"separator":      theme.Colors.Separator,
		"activity":       theme.Colors.Activity,
		"background":     theme.Colors.Background,
		"surface":        theme.Colors.Surface,
		"agent_success":  theme.Colors.AgentSuccess,
		"agent_fail":     theme.Colors.AgentFail,
	}

	for field, value := range colorFields {
		// Agent colors are optional for backwards compatibility
		if (field == "agent_success" || field == "agent_fail") && value == "" {
			continue
		}
		if err := validateColor(value, field); err != nil {
			return err
		}
	}

	return nil
}

func getDefaultTheme() *Theme {
	return &Theme{
		Name:        "Default",
		Description: "Built-in fallback theme",
		Colors: struct {
			Primary       string `yaml:"primary"`
			Secondary     string `yaml:"secondary"`
			Success       string `yaml:"success"`
			Warning       string `yaml:"warning"`
			Error         string `yaml:"error"`
			TextPrimary   string `yaml:"text_primary"`
			TextSecondary string `yaml:"text_secondary"`
			TextMuted     string `yaml:"text_muted"`
			Prompt        string `yaml:"prompt"`
			Separator     string `yaml:"separator"`
			Activity      string `yaml:"activity"`
			Background    string `yaml:"background"`
			Surface       string `yaml:"surface"`
			AgentSuccess  string `yaml:"agent_success"`
			AgentFail     string `yaml:"agent_fail"`
		}{
			Primary:       "#00AAFF",
			Secondary:     "#888888",
			Success:       "#00AA00",
			Warning:       "#FFAA00",
			Error:         "#FF0000",
			TextPrimary:   "#FFFFFF",
			TextSecondary: "#CCCCCC",
			TextMuted:     "#888888",
			Prompt:        "#00AAFF",
			Separator:     "#00AAFF",
			Activity:      "#FFFFFF",
			Background:    "#000000",
			Surface:       "#111111",
			AgentSuccess:  "#00AA00",
			AgentFail:     "#FF0000",
		},
	}
}

func LoadTheme(name string) (*Theme, error) {
	if name == "default" {
		return getDefaultTheme(), nil
	}
	if !validThemeNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid theme name '%s': only alphanumeric, '-', and '_' characters are allowed", name)
	}
	path := fmt.Sprintf(".kairon/themes/%s.yaml", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("theme '%s' not found: %w", name, err)
	}

	var theme Theme
	if err := yaml.Unmarshal(data, &theme); err != nil {
		return nil, fmt.Errorf("failed to parse theme '%s': %w", name, err)
	}

	if err := validateTheme(&theme); err != nil {
		return nil, fmt.Errorf("theme '%s' validation failed: %w", name, err)
	}

	return &theme, nil
}

func GetAvailableThemes() []string {
	var themes []string

	entries, err := os.ReadDir(".kairon/themes")
	if err != nil {
		return themes
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			name := strings.TrimSuffix(entry.Name(), ".yaml")
			themes = append(themes, name)
		}
	}

	sort.Strings(themes)
	return themes
}
