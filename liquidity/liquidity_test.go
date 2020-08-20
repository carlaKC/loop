package liquidity

import (
	"context"
	"testing"

	"github.com/lightninglabs/loop/test"
	"github.com/stretchr/testify/require"
)

// TestUpdateParameters tests runtime update of our config parameters.
func TestUpdateParameters(t *testing.T) {
	defer test.Guard(t)()

	c := newTestContext(t)
	ctx := context.Background()

	c.run()

	// First, we query with no parameters to check that we get the correct
	// error.
	_, err := c.manager.UpdateParameters(ctx, nil)
	require.Equal(t, err, ErrNoParameters)

	// Create a set of parameters we will set.
	expectCfg := &Parameters{
		IncludePrivate: false,
	}

	// Next, we check setting of non-nil parameters.
	cfg, err := c.manager.UpdateParameters(ctx, expectCfg)
	require.NoError(t, err)
	require.Equal(t, expectCfg, cfg)

	// Update a value in our parameters and update them again.
	expectCfg.IncludePrivate = true
	cfg, err = c.manager.UpdateParameters(ctx, expectCfg)
	require.NoError(t, err)
	require.Equal(t, expectCfg, cfg)

	// To test that our caller cannot malleate our parameters, we update
	// our value on our referenced struct and perform a lookup to ensure
	// that the manager is unaffected.
	expectCfg.IncludePrivate = false

	cfg, err = c.manager.UpdateParameters(ctx, nil)
	require.NoError(t, err)
	require.True(t, cfg.IncludePrivate)

	// Reset the value on our expected config so that it matches our current
	// state.
	expectCfg.IncludePrivate = true

	// Try to set parameters where one entry is invalid.
	invalidCfg := &Parameters{
		NodeRule:       NewRatioRule(1, 1),
		PeerRule:       nil,
		IncludePrivate: false,
	}
	_, err = c.manager.UpdateParameters(ctx, invalidCfg)
	require.Equal(t, ErrInvalidRatioSum, err)

	// Try to set parameters where the individual entities are valid, but as
	// a whole the config remains invalid.
	invalidCfg.NodeRule = NewRatioRule(0.2, 0.2)
	invalidCfg.PeerRule = NewRatioRule(0.2, 0.2)

	_, err = c.manager.UpdateParameters(ctx, invalidCfg)
	require.Equal(t, ErrSingleRule, err)

	// Finally, check that neither of these invalid parameter sets were
	// set.
	cfg, err = c.manager.UpdateParameters(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, expectCfg, cfg)

	// Shutdown the manager.
	c.waitForFinished()
}
