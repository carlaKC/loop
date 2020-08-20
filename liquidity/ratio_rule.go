package liquidity

// shouldSwap examines our current set of balances, and required thresholds and
// determines whether we can improve our liquidity balance. It returns an enum
// that indicates the action that should be taken based on these requirements
// and a reason enum which further explain the reasoning for this action.
func shouldSwap(balances *balances, minInbound, minOutbound float32) (Action,
	Reason) {

	// If we have no channels open (which no capacity indicates), we cannot
	// recommend any loops.
	if balances.capacity == 0 {
		return ActionNone, ReasonNoCapacity
	}

	currentInbound := balances.incomingRatio()
	currentOutbound := balances.outgoingRatio()
	inboundDeficit := currentInbound < minInbound
	outboundDeficit := currentOutbound < minOutbound

	switch {
	// If we have too little inbound and too little outbound, we cannot
	// do anything to help our liquidity situation. This will happen in the
	// case where we have a lot of pending htlcs on our channels.
	case inboundDeficit && outboundDeficit:
		return ActionNone, ReasonNoSurplus

	// If we have too little inbound, but not too little outbound, it is
	// possible that a loop out will improve our inbound liquidity.
	case inboundDeficit && !outboundDeficit:
		return ActionLoopOut, ReasonImbalanced

	// If we have enough inbound, and too little outbound, it is possible
	// that we can loop in to improve our outbound liquidity.
	case !inboundDeficit && outboundDeficit:
		return ActionLoopIn, ReasonImbalanced

	// If we have enough inbound and enough outbound, we do not need to
	// take any actions at present.
	default:
		return ActionNone, ReasonLiquidityOk
	}
}

// calculateSwapRatio calculates the ratio of capacity that we need to shift
// to improve the balance of our channels, and returns a reason which provides
// additional context. This function is used in the case where we have surplus
// liquidity in one direction, and deficit in another. This is the case in which
// we recommend swaps, if we have deficits on both sides, we cannot swap without
// further unbalancing, and if we have surplus on both sides, we do not need to.
//
// This function calculates the portion of our total capacity we should shift
// from the surplus side to deficit side without unbalancing the surplus side.
// This is important, because we do not want to recommend a swap in one
// direction that just results in our needing to produce a swap in the other
// direction.
//
// If we do have enough surplus, we aim to fall in the midpoint of our two
// minimums so that we do not tip our channel balance too far in one direction
// or the other.
func calculateSwapRatio(deficitCurrent, deficitMinimum,
	surplusCurrent, surplusMinimum float32) float32 {

	// Get the minimum shift for our deficit side that we need to swap to
	// reach our required threshold.
	required := deficitMinimum - deficitCurrent

	// Get the maximum shift allowed on our surplus side that is possible
	// before it dips beneath its minimum.
	available := surplusCurrent - surplusMinimum

	// If the amount of surplus we have is less than the minimum amount we
	// need to address our deficit, we split the excess between our
	// directions so that we move towards our desired ratio, even if we
	// can't reach our threshold.
	if available <= required {
		return available / 2
	}

	// If we have more surplus than we need to reach our deficit's minimum
	// required ratio, we need to decide how much of our channel balance we
	// want to shift. First, we get the midpoint between our two minimum
	// points and calculate the amount of balance we need to shift our
	// current deficit side to reach this midpoint.
	midpoint := (deficitMinimum + (1 - surplusMinimum)) / 2
	midpointDelta := midpoint - deficitCurrent

	// If our available surplus is sufficient to shift our deficit side all
	// the way to the midpoint between our channels, we will shift this
	// amount of balance. This ensures that the amount of balance we shift
	// never overshoots and unbalances us in the opposite direction; we
	// will always at most reach the midpoint of our two thresholds.
	if midpointDelta <= available {
		return midpointDelta
	}

	// If our available surplus is insufficient to shift our deficit side
	// all the way to this midpoint, we will just shift the minimum amount
	// we need to reach our threshold.
	return required
}
