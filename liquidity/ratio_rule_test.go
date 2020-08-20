package liquidity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShouldSwap tests assessing of a set of balances to determine whether we
// should perform a swap.
func TestShouldSwap(t *testing.T) {
	tests := []struct {
		name        string
		minIncoming float32
		minOutgoing float32
		balances    *balances
		action      Action
		reason      Reason
	}{
		{
			name: "no capacity",
			balances: &balances{
				capacity: 0,
			},
			action: ActionNone,
			reason: ReasonNoCapacity,
		},
		{
			name: "insufficient surplus",
			balances: &balances{
				capacity: 100,
				incoming: 20,
				outgoing: 20,
			},
			minOutgoing: 0.4,
			minIncoming: 0.4,
			action:      ActionNone,
			reason:      ReasonNoSurplus,
		},
		{
			name: "loop out",
			balances: &balances{
				capacity: 100,
				incoming: 20,
				outgoing: 80,
			},
			minOutgoing: 0.2,
			minIncoming: 0.6,
			action:      ActionLoopOut,
			reason:      ReasonImbalanced,
		},
		{
			name: "loop in",
			balances: &balances{
				capacity: 100,
				incoming: 50,
				outgoing: 50,
			},
			minOutgoing: 0.6,
			minIncoming: 0.3,
			action:      ActionLoopIn,
			reason:      ReasonImbalanced,
		},
		{
			name: "liquidity ok",
			balances: &balances{
				capacity: 100,
				incoming: 50,
				outgoing: 50,
			},
			minOutgoing: 0.4,
			minIncoming: 0.4,
			action:      ActionNone,
			reason:      ReasonLiquidityOk,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			action, reason := shouldSwap(
				test.balances, test.minIncoming,
				test.minOutgoing,
			)
			require.Equal(t, test.action, action)
			require.Equal(t, test.reason, reason)
		})
	}
}

// TestCalculateSwapRatio tests calculation of the ratio of our capacity that
// we need to shift to reach our liquidity targets.
func TestCalculateSwapRatio(t *testing.T) {
	tests := []struct {
		name string

		deficitCurrent float32
		deficitMinimum float32
		surplusCurrent float32
		surplusMinimum float32

		expectedRatio float32
	}{
		{
			// We have enough balance to hit our target between our
			// two ratios.
			// start: 	| 100% out         |
			// end: 	| 45% out | 55% in |
			name:           "can reach midpoint",
			deficitCurrent: 0,
			deficitMinimum: 0.3,
			surplusCurrent: 1,
			surplusMinimum: 0.2,
			expectedRatio:  0.55,
		},
		{
			// We have some pending htlcs, so we cannot shift to
			// the midpoint between our minimums, however, we still
			// have enough to reach our minimum.
			// start: 	| 70% out | 20% pending | 10 % in |
			// end: 	| 30% out | 20% pending | 50 % in |
			name:           "sufficient for minimum",
			deficitCurrent: 0.1,
			deficitMinimum: 0.5,
			surplusCurrent: 0.7,
			surplusMinimum: 0.2,
			expectedRatio:  0.4,
		},
		{
			// We have a lot of pending htlcs. If we were to shift
			// to our minimum of 50% inbound, that would unbalance
			// our outbound liquidity. We split our available
			// surplus so that we at least move in the direction
			// of our required ratio.
			// start: 	| 60% out | 30% pending | 10 % in |
			// end: 	| 45% out | 30% pending | 25 % in |
			name:           "can't reach minimum, split",
			deficitCurrent: 0.1,
			deficitMinimum: 0.5,
			surplusCurrent: 0.6,
			surplusMinimum: 0.3,
			expectedRatio:  0.15,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ratio := calculateSwapRatio(
				testCase.deficitCurrent, testCase.deficitMinimum,
				testCase.surplusCurrent, testCase.surplusMinimum,
			)
			require.Equal(t, testCase.expectedRatio, ratio)
		})
	}
}
