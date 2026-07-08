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
	// Claude model IDs (as reported by API). Lookup prefers the LONGEST
	// matching prefix, so "claude-opus-4-8" wins over "claude-opus-4" for
	// dated IDs like claude-opus-4-8-YYYYMMDD.
	"claude-fable-5":    {10.0, 50.0},
	"claude-mythos-5":   {10.0, 50.0},
	"claude-opus-4-8":   {5.0, 25.0},
	"claude-opus-4-7":   {5.0, 25.0},
	"claude-opus-4-6":   {5.0, 25.0},
	"claude-opus-4-5":   {5.0, 25.0},
	"claude-opus-4":     {15.0, 75.0}, // Opus 4.0/4.1
	"claude-sonnet-5":   {3.0, 15.0},
	"claude-sonnet-4":   {3.0, 15.0},
	"claude-haiku-4":    {0.80, 4.0},
	"claude-haiku-4-5":  {1.0, 5.0},
	"claude-3-opus":     {15.0, 75.0},
	"claude-3-sonnet":   {3.0, 15.0},
	"claude-3-haiku":    {0.25, 1.25},
	"claude-3.5-sonnet": {3.0, 15.0},
	"claude-3.5-haiku":  {0.80, 4.0},
	// Legacy naming
	"claude-4-opus":   {15.0, 75.0},
	"claude-4-sonnet": {3.0, 15.0},
	// OpenAI
	"gpt-4o":      {2.5, 10.0},
	"gpt-4o-mini": {0.15, 0.6},
	"gpt-5":       {2.5, 10.0},
	"o3":          {10.0, 40.0},
	"o4-mini":     {1.1, 4.4},
	"codex":       {3.0, 15.0},
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
	// Longest matching prefix wins: "claude-opus-4-8" must beat
	// "claude-opus-4" for dated IDs, and map iteration order is random.
	lower := strings.ToLower(model)
	best := ""
	var bestPricing modelPricing
	for prefix, p := range pricing {
		if strings.HasPrefix(lower, prefix) && len(prefix) > len(best) {
			best = prefix
			bestPricing = p
		}
	}
	return bestPricing, best != ""
}
