package liquidity

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightninglabs/loop/test"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

var (
	testTime = time.Date(2020, 02, 13, 0, 0, 0, 0, time.UTC)

	testSeverFee btcutil.Amount = 1
	testPrepay   btcutil.Amount = 2
	testMinerFee btcutil.Amount = 3
)

// newTestConfig creates a default test config.
func newTestConfig() *Config {
	return &Config{
		LoopOutRestrictions: func(_ context.Context) (*Restrictions,
			error) {

			return NewRestrictions(1, 10000), nil
		},
		Lnd:   test.NewMockLnd().Client,
		Clock: clock.NewTestClock(testTime),
		LoopOutQuote: func(context.Context, btcutil.Amount, int32) (
			btcutil.Amount, btcutil.Amount, btcutil.Amount, error) {

			return testSeverFee, testMinerFee, testPrepay, nil
		},
		ListSwaps: func(context.Context) ([]ExistingSwap, error) {
			return nil, nil
		},
	}
}

// TestParameters tests getting and setting of parameters for our manager.
func TestParameters(t *testing.T) {
	manager := NewManager(newTestConfig())

	chanID := lnwire.NewShortChanIDFromInt(1)

	// Start with the case where we have no rules set.
	startParams := manager.GetParameters()
	require.Equal(t, newParameters(), startParams)

	// Mutate the parameters returned by our get function.
	startParams.ChannelRules[chanID] = NewThresholdRule(1, 1)

	// Make sure that we have not mutated the liquidity manager's params
	// by making this change.
	params := manager.GetParameters()
	require.Equal(t, newParameters(), params)

	// Provide a valid set of parameters and validate assert that they are
	// set.
	originalRule := NewThresholdRule(10, 10)
	expected := newParameters()
	expected.ChannelRules[chanID] = originalRule

	err := manager.SetParameters(expected)
	require.NoError(t, err)

	// Check that changing the parameters we just set does not mutate
	// our liquidity manager's parameters.
	expected.ChannelRules[chanID] = NewThresholdRule(11, 11)

	params = manager.GetParameters()
	require.NoError(t, err)
	require.Equal(t, originalRule, params.ChannelRules[chanID])

	// Set invalid parameters and assert that we fail.
	expected.ChannelRules = map[lnwire.ShortChannelID]*ThresholdRule{
		lnwire.NewShortChanIDFromInt(0): NewThresholdRule(1, 2),
	}
	err = manager.SetParameters(expected)
	require.Equal(t, ErrZeroChannelID, err)
}

// TestSuggestSwaps tests getting of swap suggestions.
func TestSuggestSwaps(t *testing.T) {
	var (
		chanID1 = lnwire.NewShortChanIDFromInt(1)
		chanID2 = lnwire.NewShortChanIDFromInt(2)
	)

	tests := []struct {
		name       string
		channels   []lndclient.ChannelInfo
		parameters Parameters
		swaps      []*LoopOutRecommendation
	}{
		{
			name:       "no rules",
			channels:   nil,
			parameters: newParameters(),
		},
		{
			name: "loop out",
			channels: []lndclient.ChannelInfo{
				{
					ChannelID:     1,
					Capacity:      1000,
					LocalBalance:  1000,
					RemoteBalance: 0,
				},
			},
			parameters: Parameters{
				MaximumSwapFeePPM: DefaultSwapFeePPM,
				MaximumPrepay:     DefaultPrepay,
				MaximumMinerFee:   DefaultMinerFee,
				ConfTarget:        DefaultConfTarget,
				ChannelRules: map[lnwire.ShortChannelID]*ThresholdRule{
					chanID1: NewThresholdRule(
						10, 10,
					),
				},
			},
			swaps: []*LoopOutRecommendation{
				{
					Channel: chanID1,
					Amount:  500,
				},
			},
		},
		{
			name: "no rule for channel",
			channels: []lndclient.ChannelInfo{
				{
					ChannelID:     1,
					Capacity:      1000,
					LocalBalance:  0,
					RemoteBalance: 1000,
				},
			},
			parameters: Parameters{
				MaximumSwapFeePPM: DefaultSwapFeePPM,
				MaximumPrepay:     DefaultPrepay,
				MaximumMinerFee:   DefaultMinerFee,
				ConfTarget:        DefaultConfTarget,
				ChannelRules: map[lnwire.ShortChannelID]*ThresholdRule{
					chanID2: NewThresholdRule(10, 10),
				},
			},
			swaps: nil,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			cfg := newTestConfig()

			// Create a mock lnd with the set of channels set in our
			// test case.
			mock := test.NewMockLnd()
			mock.Channels = testCase.channels
			cfg.Lnd = mock.Client

			manager := NewManager(cfg)

			// Set our test case parameters.
			err := manager.SetParameters(testCase.parameters)
			require.NoError(t, err)

			swaps, err := manager.SuggestSwaps(context.Background())
			require.NoError(t, err)
			require.Equal(t, testCase.swaps, swaps)
		})
	}
}

// TestEligibleChannels tests selection of a set of channels that can be used
// for automated swaps.
func TestEligibleChannels(t *testing.T) {
	var (
		chanID1 = lnwire.NewShortChanIDFromInt(1)
		chanID2 = lnwire.NewShortChanIDFromInt(2)

		peer1 = route.Vertex{1}
		peer2 = route.Vertex{2}

		channel1 = lndclient.ChannelInfo{
			ChannelID:   chanID1.ToUint64(),
			PubKeyBytes: peer1,
		}

		channel2 = lndclient.ChannelInfo{
			ChannelID:   chanID2.ToUint64(),
			PubKeyBytes: peer2,
		}
	)

	tests := []struct {
		name     string
		swaps    []ExistingSwap
		channels []lndclient.ChannelInfo
		eligible []lndclient.ChannelInfo
	}{
		{
			name: "no existing swaps",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: nil,
			eligible: []lndclient.ChannelInfo{
				channel1, channel2,
			},
		},
		{
			name: "unrestricted loop out",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeOut,
				},
			},
			eligible: nil,
		},
		{
			name: "unrestricted loop in",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeIn,
				},
			},
			eligible: nil,
		},
		{
			name: "restricted loop out",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeOut,
					Channels: []lnwire.ShortChannelID{
						chanID1,
					},
				},
			},
			eligible: []lndclient.ChannelInfo{
				channel2,
			},
		},
		{
			name: "restricted loop in",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeIn,
					Peer: &peer2,
				},
			},
			eligible: []lndclient.ChannelInfo{
				channel1,
			},
		},
		{
			name: "swap failed recently",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeOut,
					Channels: []lnwire.ShortChannelID{
						chanID1,
					},
					State:      loopdb.StateFailOffchainPayments,
					LastUpdate: testTime,
				},
			},
			eligible: []lndclient.ChannelInfo{
				channel2,
			},
		},
		{
			name: "swap failed before cutoff",
			channels: []lndclient.ChannelInfo{
				channel1, channel2,
			},
			swaps: []ExistingSwap{
				{
					Type: swap.TypeOut,
					Channels: []lnwire.ShortChannelID{
						chanID1,
					},
					State: loopdb.StateFailOffchainPayments,
					LastUpdate: testTime.Add(
						DefaultFailureBackOff * -1,
					),
				},
			},
			eligible: []lndclient.ChannelInfo{
				channel1, channel2,
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newTestConfig()

			// Create a mock lnd with the set of channels set in our
			// test case.
			mock := test.NewMockLnd()
			mock.Channels = testCase.channels
			cfg.Lnd = mock.Client

			manager := NewManager(cfg)

			actual, err := manager.getEligibleChannels(
				context.Background(), testCase.swaps,
			)
			require.NoError(t, err)
			require.Equal(t, testCase.eligible, actual)
		})
	}
}

// TestCheckFeeLimits tests checking of fee limits against a quote returned by
// the server.
func TestCheckFeeLimits(t *testing.T) {
	tests := []struct {
		name                      string
		amount                    btcutil.Amount
		params                    Parameters
		minerFee, swapFee, prepay btcutil.Amount
		err                       error
	}{
		{
			name:   "fees ok",
			amount: 100000,
			params: Parameters{
				MaximumPrepay:     100,
				MaximumSwapFeePPM: 5000,
				MaximumMinerFee:   500,
				ConfTarget:        DefaultConfTarget,
			},
			minerFee: 50,
			swapFee:  1,
			prepay:   10,
			err:      nil,
		},
		{
			name:   "insufficient prepay",
			amount: 100000,
			params: Parameters{
				MaximumPrepay:     100,
				MaximumSwapFeePPM: 5000,
				MaximumMinerFee:   500,
				ConfTarget:        DefaultConfTarget,
			},
			minerFee: 50,
			swapFee:  1,
			prepay:   200,
			err:      errInsufficientPrepay,
		},
		{
			name:   "insufficient miner fee",
			amount: 100000,
			params: Parameters{
				MaximumPrepay:     100,
				MaximumSwapFeePPM: 5000,
				MaximumMinerFee:   500,
				ConfTarget:        DefaultConfTarget,
			},
			minerFee: 600,
			swapFee:  1,
			prepay:   100,
			err:      errInsufficientMinerFee,
		},
		{
			name:   "insufficient swap fee",
			amount: 100000,
			params: Parameters{
				MaximumPrepay:     100,
				MaximumSwapFeePPM: 5000,
				MaximumMinerFee:   500,
				ConfTarget:        DefaultConfTarget,
			},
			minerFee: 500,
			swapFee:  501,
			prepay:   100,
			err:      errInsufficientSwapFee,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create a manager and set our test parameters.
			mgr := NewManager(newTestConfig())
			err := mgr.SetParameters(testCase.params)
			require.NoError(t, err)

			suggestion := newLoopOutRecommendation(
				testCase.amount,
				lnwire.NewShortChanIDFromInt(1),
			)

			err = mgr.checkFeeLimits(
				testCase.swapFee, testCase.minerFee,
				testCase.prepay, suggestion,
			)
			require.Equal(t, testCase.err, err)
		})
	}
}
