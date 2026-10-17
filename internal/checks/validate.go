package checks

import "fmt"

// ValidateOptions configures Validate.
type ValidateOptions struct {
	// HiddenDir is where `inject` sources are resolved ("" = no inject allowed).
	HiddenDir string
	// Criterion, when non-nil, is called with every check's criterion and
	// returns an error to reject it (eval passes the rubric cross-check). nil
	// skips the cross-check.
	Criterion func(name string) error
	// OptionalCriterion lets a check omit `criterion` (kairon check has no
	// rubric). When false, a missing criterion is the error "criterion is
	// required".
	OptionalCriterion bool
}

// Validate validates checks loaded from `file` and compiles them in place: it
// compiles patterns and allow globs, resolves inject sources against
// opts.HiddenDir and defaults expect_exit to 0 for command checks. Every error
// wraps ErrInvalid and names the file, the 1-based check index and the check
// type.
func Validate(file string, checks []Check, opts ValidateOptions) error {
	for i := range checks {
		if err := validateCheck(&checks[i], opts); err != nil {
			typ := string(checks[i].Type)
			if typ == "" {
				typ = "?"
			}
			return fmt.Errorf("%w: %s: check #%d (%s): %s", ErrInvalid, file, i+1, typ, err)
		}
	}
	return nil
}
