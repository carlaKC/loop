package liquidity

// Action represents the action that our manager recommends.
type Action uint8

const (
	// ActionNone indicates that no action is recommended at present.
	ActionNone Action = iota

	// ActionLoopOut indicates that looping out to acquire inbound liquidity
	// is recommended.
	ActionLoopOut

	// ActionLoopIn indicates that looping in to acquire outbound liquidity
	// is recommended.
	ActionLoopIn
)

// String returns the string representation of an action.
func (a Action) String() string {
	switch a {
	case ActionNone:
		return "No action"

	case ActionLoopOut:
		return "Loop out"

	case ActionLoopIn:
		return "Loop in"

	default:
		return "unknown"
	}
}

// Reason provides additional reasoning for the action that we recommend.
type Reason uint8

const (
	// ReasonImbalanced is returned when our channels are below our required
	// threshold in one direction, and have sufficient surplus in the other
	// direction for us to rebalance.
	ReasonImbalanced Reason = iota

	// ReasonNoCapacity indicates that we have no channel capacity that is
	// eligible for swaps. This may be the case if we have no channels, or
	// only have private channels and are excluding them.
	ReasonNoCapacity

	// ReasonNoSurplus indicates that we have no surplus on either side of
	// the channel. We cannot perform a swap to acquire liquidity in one
	// direction, because that would unbalance the other direction. This
	// may be the case when we have many pending htlcs on a channel.
	ReasonNoSurplus

	// ReasonLiquidityOk indicates that our inbound and outbound are at
	// acceptable levels, so we do not need to perform any swaps.
	ReasonLiquidityOk

	// ReasonMinimumAmount indicates that we recommend performing a swap,
	// but the amount that we need to swap is less than the minimum swap
	// amount.
	ReasonMinimumAmount
)

// String returns the string representation of an observation.
func (r Reason) String() string {
	switch r {
	case ReasonImbalanced:
		return "Channels imbalanced"

	case ReasonNoCapacity:
		return "No capacity"

	case ReasonNoSurplus:
		return "No surplus"

	case ReasonLiquidityOk:
		return "Liquidity ok"

	case ReasonMinimumAmount:
		return "Imbalance amount less than minimum swap amount"

	default:
		return "unknown"
	}
}
