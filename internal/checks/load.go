package checks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a standalone checks file: a top-level YAML sequence of the
// documented check mappings (the value of a case's `checks:` key).
//
// The file must be a regular file of at most 10 MiB. Decoding is strict
// (unknown keys are rejected, as in case files) and an empty list is an
// error, so a guardrail that checks nothing can never pass. LoadFile does not
// validate the checks; call Validate on the result. Every error names path.
func LoadFile(path string) ([]Check, error) {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, fmt.Errorf("checks file %s: %w", path, err)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("checks file %s: not a regular file", path)
	case info.Size() > maxCheckFileBytes:
		return nil, fmt.Errorf("checks file %s: larger than the 10 MiB limit (%d bytes)", path, info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("checks file %s: %w", path, err)
	}
	defer f.Close()
	// Bound the read even if the file grew after the Stat above.
	data, err := io.ReadAll(io.LimitReader(f, maxCheckFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("checks file %s: %w", path, err)
	}
	if len(data) > maxCheckFileBytes {
		return nil, fmt.Errorf("checks file %s: larger than the 10 MiB limit", path)
	}

	var list []Check
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&list); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("checks file %s: %w", path, err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no checks in %s", path)
	}
	// The whole file is validated before any check runs, so a trailing YAML
	// document must not be silently ignored. Only a single document (the next
	// Decode returns io.EOF) is accepted.
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("checks file %s: multiple YAML documents", path)
		}
		return nil, fmt.Errorf("checks file %s: %w", path, err)
	}
	return list, nil
}
