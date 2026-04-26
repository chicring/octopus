package anthropic

// thinkingBudgetToReasoningEffort converts thinking budget tokens to reasoning effort string.
func thinkingBudgetToReasoningEffort(budgetTokens int64) string {
	// Map budget tokens to reasoning effort based on the same logic used in outbound
	if budgetTokens <= 5000 {
		return EffortLow
	} else if budgetTokens <= 15000 {
		return EffortMedium
	} else if budgetTokens <= 32768 {
		return EffortHigh
	} else if budgetTokens <= 65536 {
		return EffortXHigh
	} else {
		return EffortMax
	}
}

// getDefaultReasoningEffortMapping returns the default mapping from ReasoningEffort to thinking budget tokens.
var defaultReasoningEffortMapping = map[string]int64{
	EffortLow:    5000,
	EffortMedium: 15000,
	EffortHigh:   32768,
	EffortXHigh:  65536,
	EffortMax:    131072,
}
