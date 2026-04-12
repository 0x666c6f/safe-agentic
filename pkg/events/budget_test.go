package events

import "testing"

func TestCheckBudgetUnder(t *testing.T) {
	if CheckBudget(1.50, 5.00) {
		t.Error("cost 1.50 < budget 5.00: expected false (not exceeded)")
	}
}

func TestCheckBudgetAtExact(t *testing.T) {
	if CheckBudget(5.00, 5.00) {
		t.Error("cost 5.00 == budget 5.00: expected false (not exceeded)")
	}
}

func TestCheckBudgetOver(t *testing.T) {
	if !CheckBudget(5.01, 5.00) {
		t.Error("cost 5.01 > budget 5.00: expected true (exceeded)")
	}
}

func TestCheckBudgetZeroBudget(t *testing.T) {
	if !CheckBudget(0.01, 0.00) {
		t.Error("cost 0.01 > budget 0.00: expected true (exceeded)")
	}
}

func TestCheckBudgetBothZero(t *testing.T) {
	if CheckBudget(0.00, 0.00) {
		t.Error("cost 0.00 == budget 0.00: expected false (not exceeded)")
	}
}
