package events

func CheckBudget(cost, budget float64) bool {
	return cost > budget
}
