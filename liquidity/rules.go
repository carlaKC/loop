package liquidity

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidLiquidityRatio is returned when a ratio rule has an invalid
	// value for one of its thresholds.
	ErrInvalidLiquidityRatio = errors.New("liquidity ratio must be in " +
		"[0;1]")

	// ErrInvalidRatioSum is returned when the sum of the thresholds
	// provided for a ratio rule is >= 1.
	ErrInvalidRatioSum = errors.New("sum of inbound and outbound ratios " +
		"must be < 1")
)

// Rule is an interface implemented by different liquidity rules that we can
// apply.
type Rule interface {
	fmt.Stringer

	// validate validates the parameters that a rule was created with.
	validate() error
}

// RatioRule is a liquidity rule that implements minimum incoming and outgoing
// liquidity ratios.
type RatioRule struct {
	// Minimum inbound is the minimum ratio of inbound liquidity we allow
	// before recommending a loop out to acquire incoming liquidity.
	MinimumInbound float32

	// MinimumOutbound is the minimum ratio of outbound liquidity we allow
	// before recommending a loop in to acquire outgoing liquidity.
	MinimumOutbound float32
}

// NewRatioRule returns a new ratio rule.
func NewRatioRule(minimumInbound, minimumOutbound float32) *RatioRule {
	return &RatioRule{
		MinimumInbound:  minimumInbound,
		MinimumOutbound: minimumOutbound,
	}
}

// String returns a string representation of a rule.
func (r *RatioRule) String() string {
	return fmt.Sprintf("ratio rule: minimum inbound: %v, minimum "+
		"outbound: %v", r.MinimumInbound, r.MinimumOutbound)
}

// validate validates the parameters that a rule was created with.
func (r *RatioRule) validate() error {
	if r.MinimumInbound < 0 || r.MinimumInbound > 1 {
		return ErrInvalidLiquidityRatio
	}

	if r.MinimumOutbound < 0 || r.MinimumOutbound > 1 {
		return ErrInvalidLiquidityRatio
	}

	if r.MinimumInbound+r.MinimumOutbound >= 1 {
		return ErrInvalidRatioSum
	}

	return nil
}
