package eval

import (
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
	for _, r := range recs {
		source := string(inference.UsageReported)
		if r.Estimated {
			source = string(inference.UsageEstimated)
		}
		cr.Calls = append(cr.Calls, r)
		cr.AgentCost.Add(CostInfo{
			TokensIn:     r.InputTokens,
			TokensOut:    r.OutputTokens,
			EstimatedUSD: r.CostUSD,
			Model:        r.Model,
			UsageSource:  source,
		})
	}
	return nil
}
