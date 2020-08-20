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

	// Shutdown the manager.
	c.waitForFinished()
}
