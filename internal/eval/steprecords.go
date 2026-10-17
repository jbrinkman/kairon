package eval

import (
	"fmt"
	"math"

	"github.com/jbrinkman/kairon/internal/inference"
)

// importStepRecords reads the workspace's step records and, on success,
// appends them to cr.Calls and adds their tokens and cost to cr.AgentCost. On
// error nothing is imported and cr is left unchanged.
//
// The reading and validation live in internal/checks (readStepRecords is an
// alias for checks.ReadStepRecords); this function stays in eval because it
// folds the records into eval's CaseResult/CostInfo.
func importStepRecords(cr *CaseResult, dir string) error {
	recs, err := readStepRecords(dir)
	if err != nil {
		return err
	}
	// ReadStepRecords only guards the records' own totals (from 0). The sums
	// land on cr.AgentCost, which already holds the outer agent usage, so an
	// otherwise-valid record can still wrap that total past the sign (tokens)
	// or make it non-finite (cost). Check the running total against the
	// existing AgentCost before mutating cr, keeping the import all-or-nothing
	// — a workspace must not be able to drive the case's cost negative.
	in, out := cr.AgentCost.TokensIn, cr.AgentCost.TokensOut
	usd := cr.AgentCost.EstimatedUSD
	for _, r := range recs {
		if in+r.InputTokens < in || out+r.OutputTokens < out {
			return fmt.Errorf("%s: imported tokens overflow the agent total", stepRecordsRel)
		}
		in, out = in+r.InputTokens, out+r.OutputTokens
		usd += r.CostUSD
		if math.IsInf(usd, 0) || math.IsNaN(usd) {
			return fmt.Errorf("%s: imported cost overflows the agent total", stepRecordsRel)
		}
	}
	for _, r := range recs {
		source := string(inference.UsageReported)
		if r.Estimated {
			source = string(inference.UsageEstimated)
		}
		cr.Calls = append(cr.Calls, r)
		c := CostInfo{
			TokensIn:     r.InputTokens,
			TokensOut:    r.OutputTokens,
			EstimatedUSD: r.CostUSD,
			Model:        r.Model,
			UsageSource:  source,
		}
		// A cost_unknown record contributes 0 USD only because its model has
		// no price, so the merged cost is a lower bound: mark it incomplete and
		// record the model as unpriced, exactly as costFromUsage does.
		if r.CostUnknown {
			c.CostIncomplete = true
			if r.Model != "" {
				c.UnpricedModels = []string{r.Model}
			}
		}
		cr.AgentCost.Add(c)
	}
	return nil
}
