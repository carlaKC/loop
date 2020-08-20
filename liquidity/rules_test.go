package liquidity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateConfigParams tests validation of our config parameters.
func TestValidateRatioRule(t *testing.T) {
	tests := []struct {
		name  string
		ratio RatioRule
		err   error
	}{
		{
			name: "values ok",
			ratio: RatioRule{
				MinimumInbound:  0.2,
				MinimumOutbound: 0.2,
			},
			err: nil,
		},
		{
			name: "negative inbound",
			ratio: RatioRule{
				MinimumInbound:  -1,
				MinimumOutbound: 0.2,
			},
			err: ErrInvalidLiquidityRatio,
		},
		{
			name: "negative outbound",
			ratio: RatioRule{
				MinimumInbound:  0.2,
				MinimumOutbound: -1,
			},
			err: ErrInvalidLiquidityRatio,
		},
		{
			name: "inbound > 1",
			ratio: RatioRule{
				MinimumInbound:  1.2,
				MinimumOutbound: 0.2,
			},
			err: ErrInvalidLiquidityRatio,
		},
		{
			name: "outbound >1",
			ratio: RatioRule{
				MinimumInbound:  0.2,
				MinimumOutbound: 1.2,
			},
			err: ErrInvalidLiquidityRatio,
		},
		{
			name: "sum < 1",
			ratio: RatioRule{
				MinimumInbound:  0.6,
				MinimumOutbound: 0.39,
			},
			err: nil,
		},
		{
			name: "sum = 1",
			ratio: RatioRule{
				MinimumInbound:  0.6,
				MinimumOutbound: 0.4,
			},
			err: ErrInvalidRatioSum,
		},
		{
			name: "sum > 1",
			ratio: RatioRule{
				MinimumInbound:  0.6,
				MinimumOutbound: 0.6,
			},
			err: ErrInvalidRatioSum,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.ratio.validate()
			require.Equal(t, testCase.err, err)
		})
	}
}
