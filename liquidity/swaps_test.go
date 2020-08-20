package liquidity

import (
	"testing"

	"github.com/btcsuite/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// TestSelectSwaps tests selection of Swaps from a set of channels.
func TestSelectSwaps(t *testing.T) {
	var (
		chan1 = lnwire.NewShortChanIDFromInt(1)
		chan2 = lnwire.NewShortChanIDFromInt(2)
	)

	tests := []struct {
		name      string
		channels  []channelSurplus
		amount    btcutil.Amount
		minAmount btcutil.Amount
		maxAmount btcutil.Amount
		swaps     []SwapRecommendation
	}{
		{
			name: "minimum amount exactly required",
			channels: []channelSurplus{
				{
					channel: chan1,
					amount:  10,
				},
				{
					channel: chan2,
					amount:  5,
				},
			},
			amount:    10,
			minAmount: 10,
			maxAmount: 100,
			swaps: []SwapRecommendation{
				{
					Channel: chan1,
					Amount:  10,
				},
			},
		},
		{
			name: "enough balance, but below minimum",
			channels: []channelSurplus{
				{
					channel: chan1,
					amount:  5,
				},
				{
					channel: chan2,
					amount:  5,
				},
			},
			amount:    10,
			minAmount: 10,
			maxAmount: 100,
			swaps:     nil,
		},
		{
			name: "more available than required",
			channels: []channelSurplus{
				{
					channel: chan1,
					amount:  50,
				},
			},
			amount:    20,
			minAmount: 10,
			maxAmount: 100,
			swaps: []SwapRecommendation{
				{
					Channel: chan1,
					Amount:  20,
				},
			},
		},
		{
			name: "more available than required, multiple swaps",
			channels: []channelSurplus{
				{
					channel: chan1,
					amount:  200,
				},
				{
					channel: chan2,
					amount:  200,
				},
			},
			amount:    150,
			minAmount: 10,
			maxAmount: 100,
			swaps: []SwapRecommendation{
				{
					Channel: chan1,
					Amount:  100,
				},
				{
					Channel: chan2,
					Amount:  50,
				},
			},
		},
		{
			name: "cannot get exact amount",
			channels: []channelSurplus{
				{
					channel: chan1,
					amount:  20,
				},
				{
					channel: chan2,
					amount:  20,
				},
			},
			amount:    25,
			minAmount: 10,
			maxAmount: 100,
			swaps: []SwapRecommendation{
				{
					Channel: chan1,
					Amount:  20,
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			swaps := selectSingleSwap(
				test.channels, test.amount, test.minAmount,
				test.maxAmount,
			)
			require.Equal(t, test.swaps, swaps)
		})
	}
}
