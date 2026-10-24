## Repository Context

Excerpts from the repository the planner may find while investigating an issue. Each excerpt is quoted as
`path:line`, so a conclusion drawn from it can cite a real file and line.

### Exporter conventions

`internal/export/doc.go:3`
```go
// Package export writes run records to an io.Writer in one file format per exporter.
// Every exporter implements Exporter and is registered in registry.go.
```

`internal/export/exporter.go:11`
```go
// Exporter writes records to w. Implementations must not buffer past the call:
// when Export returns, everything has been written to w.
type Exporter interface {
	Name() string
	Export(w io.Writer, records []Record) error
}
```

`internal/export/csv.go:19`
```go
func (CSVExporter) Export(w io.Writer, records []Record) error {
	// Empty input is a no-op: write nothing, not even a header row, and return nil.
	if len(records) == 0 {
		return nil
	}
```

`internal/export/markdown.go:22`
```go
func (MarkdownExporter) Export(w io.Writer, records []Record) error {
	// Empty input is a no-op: write nothing and return nil.
	if len(records) == 0 {
		return nil
	}
```

`internal/export/csv.go:41`
```go
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("export csv: record %d: %w", i, err)
		}
```

`internal/export/registry.go:9`
```go
// exporters is the set of known exporters, keyed by Name(). The CLI flag
// --format selects one of these keys. An unknown key is an error that lists the valid keys.
var exporters = map[string]Exporter{
	"csv":      CSVExporter{},
	"markdown": MarkdownExporter{},
}
```

`internal/export/record.go:8`
```go
// Record is one finished agent run.
type Record struct {
	Issue     int           `json:"issue"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
}
```

`internal/export/csv_test.go:14`
```go
func TestCSVExporterEmptyInput(t *testing.T) { // table-driven; one case per exporter behavior
```

### CLI wiring

`cmd/kairon/export.go:27`
```go
	issues := fs.String("issues", "", "comma-separated issue numbers to export")
	format := fs.String("format", "csv", "export format: one of the registered exporters")
	out := fs.String("out", "", "output file path (default: stdout)")
```

`cmd/kairon/export.go:44`
```go
	if err := exp.Export(dst, records); err != nil {
		return fmt.Errorf("export: %w", err) // errors are returned to main, which prints them and exits 1
	}
```

### Configuration

`.kairon/config.yaml:1`
```yaml
repo: owner/repo-name
label: kairon
poll_interval: 5m
max_retries: 3
```

### Related code path: run history lookup

`internal/export/history.go:31`
```go
// Load returns records for the given issue numbers.
func Load(dir string, issues []int) ([]Record, error) {
	var out []Record
	for _, n := range issues {
		recs, err := readIssue(dir, n)
		if err != nil {
			return nil, err
		}
		out = append(out, recs...)
	}
	return out, nil
}
```

`internal/export/history.go:58`
```go
func readIssue(dir string, n int) ([]Record, error) {
	b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("issue-%d.json", n)))
	if err != nil {
		return nil, err
	}
	var recs []Record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}
```

`internal/export/history.go:77`
```go
// Dedupe drops repeated issue numbers, keeping the first occurrence of each.
func Dedupe(issues []int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, n := range issues {
		if !seen[n] {
			out = append(out, n)
		}
	}
	return out
}
```

`cmd/kairon/export.go:38`
```go
	records, err := export.Load(historyDir, export.Dedupe(issues))
```
