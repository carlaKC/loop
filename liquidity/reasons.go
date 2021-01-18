package liquidity

// Reason is an enum that indicates the reason that a swap was not executed.
type Reason int

const (
	// ReasonNone indicates that there is no reason we cannot perform a
	// swap if one is required.
	ReasonNone Reason = iota

	// ReasonBudgetElapsed indicates that the liquidity budget has run out.
	ReasonBudgetElapsed

	// ReasonFeesTooHigh indicates that chain fees are too high for swaps
	// to be dispatched.
	ReasonFeesToHigh

	// ReasonBudgetConsumed is returned if we have consumed our full budget
	// and cannot perform any more swaps.
	ReasonBudgetConsumed

	// ReasonInFlightLimit indicates that we have reached our limit for
	// in-flight automatically dispatched swaps.
	ReasonInFlightLimit
)
