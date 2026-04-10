package cost

import "strings"

type TokenUsage struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
}

type modelPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

var pricing = map[string]modelPricing{
	"claude-3-opus":     {15.0, 75.0},
	"claude-3-sonnet":   {3.0, 15.0},
	"claude-3-haiku":    {0.25, 1.25},
	"claude-3.5-sonnet": {3.0, 15.0},
	"claude-3.5-haiku":  {0.80, 4.0},
	"claude-4-opus":     {15.0, 75.0},
	"claude-4-sonnet":   {3.0, 15.0},
	"gpt-4o":            {2.5, 10.0},
	"gpt-4o-mini":       {0.15, 0.6},
	"o3":                {10.0, 40.0},
	"o4-mini":           {1.1, 4.4},
	"codex":             {3.0, 15.0},
}

func ComputeCost(usage TokenUsage) float64 {
	p, ok := lookupPricing(usage.Model)
	if !ok {
		return 0
	}
	inCost := float64(usage.InputTokens) / 1_000_000.0 * p.InputPerMTok
	outCost := float64(usage.OutputTokens) / 1_000_000.0 * p.OutputPerMTok
	return inCost + outCost
}

func SumCost(usages []TokenUsage) float64 {
	total := 0.0
	for _, u := range usages {
		total += ComputeCost(u)
	}
	return total
}

func lookupPricing(model string) (modelPricing, bool) {
	if p, ok := pricing[model]; ok {
		return p, true
	}
	lower := strings.ToLower(model)
	for prefix, p := range pricing {
		if strings.HasPrefix(lower, prefix) {
			return p, true
		}
	}
	return modelPricing{}, false
}
