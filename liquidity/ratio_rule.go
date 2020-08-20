package liquidity

import (
	"fmt"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/loop/swap"
)

func (r *RatioRule) getSwaps(channelBalances []balances,
	outRestrictions, inRestrictions Restrictions) (*SwapSet, error) {

	// To decide whether we should swap, we will look at all of our balances
	// combined.
	var totalBalance balances
	for _, balance := range channelBalances {
		totalBalance.capacity += balance.capacity
		totalBalance.incoming += balance.incoming
		totalBalance.outgoing += balance.outgoing
	}

	// Examine our total balance and required ratios to decide whether we
	// need to swap.
	action, reason := shouldSwap(
		&totalBalance, r.MinimumInbound, r.MinimumOutbound,
	)

	var (
		shiftRatio   float32
		swapType     swap.Type
		restrictions Restrictions
	)

	// Switch on our observation, returning for the values that indicate
	// there is no further action. If we can perform a swap, we calculate
	// the ratio of our balance that we need to shift.
	switch action {
	case ActionNone:
		return newSwapSet(action, reason, nil), nil

	case ActionLoopOut:
		swapType = swap.TypeOut
		restrictions = outRestrictions

		shiftRatio = calculateSwapRatio(
			totalBalance.incomingRatio(), r.MinimumInbound,
			totalBalance.outgoingRatio(), r.MinimumOutbound,
		)

	case ActionLoopIn:
		swapType = swap.TypeIn
		restrictions = inRestrictions

		shiftRatio = calculateSwapRatio(
			totalBalance.outgoingRatio(), r.MinimumOutbound,
			totalBalance.incomingRatio(), r.MinimumInbound,
		)

	default:
		return nil, fmt.Errorf("unknown action: %v", action)
	}

	// At this stage, we know that we need to perform a swap, and we know
	// the ratio of our total capacity that we need to move. Before we
	// proceed, we do a quick check that the amount we need to move is more
	// than the minimum swap amount.
	amt := float32(totalBalance.capacity) * shiftRatio

	// If the amount that we need to shift is less than the minimum swap
	// amount, we cannot perform a swap yet, so we return.
	if amt < float32(restrictions.MinimumAmount) {
		return newSwapSet(action, ReasonMinimumAmount, nil), nil
	}

	// Run through our channels and get their current surplus based on the
	// direction that our swap will be in. For loop in, we look at our
	// available inbound, for loop out, we look at outbound. If a specific
	// channel does not have surplus in the required direction, we skip it.
	var channels []channelSurplus
	for _, channel := range channelBalances {
		var surplus float32

		if swapType == swap.TypeIn {
			surplus = channel.incomingRatio() - r.MinimumInbound
		} else {
			surplus = channel.outgoingRatio() - r.MinimumOutbound
		}

		if surplus <= 0 {
			continue
		}

		channels = append(channels, channelSurplus{
			amount:  btcutil.Amount(float32(channel.capacity) * surplus),
			channel: channel.channelID,
		})
	}

	// TODO(carla): add multi-swap selection for loop out, mocking the
	// behaviour of lnd's current split algorithm.
	swaps := selectSingleSwap(
		channels, btcutil.Amount(amt), restrictions.MinimumAmount,
		restrictions.MaximumAmount,
	)

	return newSwapSet(action, reason, swaps), nil
}

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
