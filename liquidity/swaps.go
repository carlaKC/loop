package liquidity

import (
	"sort"

	"github.com/btcsuite/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
)

// SwapSet describes a set of swaps that we recommend.
type SwapSet struct {
	// Action indicates the action that we decided to take based on the
	// current set of balances.
	Action

	// Reason provides further reasoning for our proposed action.
	Reason

	// Swaps is the set of swaps that we recommend.
	Swaps []SwapRecommendation
}

func newSwapSet(action Action, reason Reason,
	swaps []SwapRecommendation) *SwapSet {

	return &SwapSet{
		Action: action,
		Reason: reason,
		Swaps:  swaps,
	}
}

// SwapRecommendation contains a swap that we recommend.
type SwapRecommendation struct {
	Amount  btcutil.Amount
	Channel lnwire.ShortChannelID
}

func newSwapRecommendation(amount btcutil.Amount,
	channel lnwire.ShortChannelID) SwapRecommendation {

	return SwapRecommendation{
		Amount:  amount,
		Channel: channel,
	}
}

type channelSurplus struct {
	amount  btcutil.Amount
	channel lnwire.ShortChannelID
}

// selectSingleSwap takes a set of channels with surplus balance available and
// returns a set of recommended swaps, taking into account the size restrictions
// placed on swaps. This function assumes that we will be performing our swap
// payment with a single htlc, so does not attempt to split our amount across
// channels. It breaks our swap up into multiple swaps if the amount we require
// is more than the maximum swap size. This function will only recommend one
// swap per channel.
func selectSingleSwap(channels []channelSurplus, amount, minSwapAmount,
	maxSwapAmount btcutil.Amount) []SwapRecommendation {

	// Sort our channels from most to least available surplus.
	sort.SliceStable(channels, func(i, j int) bool {
		return channels[i].amount > channels[j].amount
	})

	var swaps []SwapRecommendation

	for _, channel := range channels {
		availableAmt := channel.amount

		// If the available amount is smaller than the minimum amount we
		// can swap, we cannot use this chanel.
		if availableAmt < minSwapAmount {
			continue
		}

		// If we have more available in this channel than we need, we
		// just aim to swap our total amount.
		if availableAmt > amount {
			availableAmt = amount
		}

		// If the surplus amount is more than our maximum amount, we
		// set our swap amount to the full surplus, otherwise we just
		// use our maximum amount.
		swapAmt := maxSwapAmount
		if availableAmt < maxSwapAmount {
			swapAmt = availableAmt
		}

		// Add a swap with this amount to our set of recommended swaps.
		swap := newSwapRecommendation(swapAmt, channel.channel)
		swaps = append(swaps, swap)

		// Subtract this swap amount from the total we will need to
		// swap.
		amount -= swapAmt

		// Once our swap amount falls under the minimum swap amount, we
		// can break our loop because we cannot swap any further.
		if amount < minSwapAmount {
			break
		}
	}

	return swaps
}
