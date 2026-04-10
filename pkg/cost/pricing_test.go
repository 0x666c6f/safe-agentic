package cost

import (
	"math"
	"testing"
)

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestComputeCostClaudeOpus(t *testing.T) {
	usage := TokenUsage{
		Model:        "claude-3-opus",
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
	}
	// 1M input @ $15/MTok = $15.00
	// 100K output @ $75/MTok = $7.50
	// total = $22.50
	got := ComputeCost(usage)
	want := 22.5
	if !almostEqual(got, want, 0.0001) {
		t.Errorf("ComputeCost(claude-3-opus, 1M in, 100K out) = %f, want %f", got, want)
	}
}

func TestComputeCostUnknownModel(t *testing.T) {
	usage := TokenUsage{
		Model:        "unknown-model-xyz",
		InputTokens:  1_000_000,
		OutputTokens: 500_000,
	}
	got := ComputeCost(usage)
	if got != 0 {
		t.Errorf("ComputeCost(unknown model) = %f, want 0", got)
	}
}

func TestSumCostTwoEntries(t *testing.T) {
	usages := []TokenUsage{
		{
			Model:        "claude-3-opus",
			InputTokens:  1_000_000,
			OutputTokens: 100_000,
		},
		{
			Model:        "gpt-4o",
			InputTokens:  500_000,
			OutputTokens: 200_000,
		},
	}
	// claude-3-opus: $15.00 + $7.50 = $22.50
	// gpt-4o: 0.5M * $2.5/MTok + 0.2M * $10/MTok = $1.25 + $2.00 = $3.25
	// total = $25.75
	got := SumCost(usages)
	want := 25.75
	if !almostEqual(got, want, 0.0001) {
		t.Errorf("SumCost(2 entries) = %f, want %f", got, want)
	}
}

func TestComputeCostPrefixMatch(t *testing.T) {
	// Model name with extra version suffix should match by prefix
	usage := TokenUsage{
		Model:        "claude-3-sonnet-20241022",
		InputTokens:  1_000_000,
		OutputTokens: 0,
	}
	// claude-3-sonnet @ $3/MTok input
	got := ComputeCost(usage)
	want := 3.0
	if !almostEqual(got, want, 0.0001) {
		t.Errorf("ComputeCost(claude-3-sonnet prefix) = %f, want %f", got, want)
	}
}

func TestComputeCostZeroTokens(t *testing.T) {
	usage := TokenUsage{
		Model:        "claude-3-haiku",
		InputTokens:  0,
		OutputTokens: 0,
	}
	got := ComputeCost(usage)
	if got != 0 {
		t.Errorf("ComputeCost(zero tokens) = %f, want 0", got)
	}
}

func TestSumCostEmpty(t *testing.T) {
	got := SumCost(nil)
	if got != 0 {
		t.Errorf("SumCost(nil) = %f, want 0", got)
	}
}
