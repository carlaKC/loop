// Package liquidity is responsible for monitoring our node's liquidity. It
// allows setting of a liquidity rule which describes the desired liquidity
// balance on a per-channel basis.
package liquidity

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

var (
	// ErrZeroChannelID is returned if we get a rule for a 0 channel ID.
	ErrZeroChannelID = fmt.Errorf("zero channel ID not allowed")
)

// Config contains the external functionality required to run the
// liquidity manager.
type Config struct {
	// LoopOutRestrictions returns the restrictions that the server applies
	// to loop out swaps.
	LoopOutRestrictions func(ctx context.Context) (*Restrictions, error)

	// ListSwaps returns the set of swaps that loop has already created.
	// These swaps may be in a final or pending state.
	ListSwaps func(ctx context.Context) ([]ExistingSwap, error)

	// Lnd provides us with access to lnd's main rpc.
	Lnd lndclient.LightningClient

	// Clock allows easy mocking of time in unit tests.
	Clock clock.Clock
}

// Parameters is a set of parameters provided by the user which guide
// how we assess liquidity.
type Parameters struct {
	// ChannelRules maps a short channel ID to a rule that describes how we
	// would like liquidity to be managed.
	ChannelRules map[lnwire.ShortChannelID]*ThresholdRule
}

// newParameters creates an empty set of parameters.
func newParameters() Parameters {
	return Parameters{
		ChannelRules: make(map[lnwire.ShortChannelID]*ThresholdRule),
	}
}

// String returns the string representation of our parameters.
func (p Parameters) String() string {
	channelRules := make([]string, 0, len(p.ChannelRules))

	for channel, rule := range p.ChannelRules {
		channelRules = append(
			channelRules, fmt.Sprintf("%v: %v", channel, rule),
		)
	}

	return fmt.Sprintf("channel rules: %v",
		strings.Join(channelRules, ","))
}

// validate checks whether a set of parameters is valid.
func (p Parameters) validate() error {
	for channel, rule := range p.ChannelRules {
		if channel.ToUint64() == 0 {
			return ErrZeroChannelID
		}

		if err := rule.validate(); err != nil {
			return fmt.Errorf("channel: %v has invalid rule: %v",
				channel.ToUint64(), err)
		}
	}

	return nil
}

// ExistingSwap provides information about a swap that has been dispatched.
type ExistingSwap struct {
	// SwapHash is the hash used for the swap.
	SwapHash lntypes.Hash

	// State is the current state of the swap.
	State loopdb.SwapState

	// Type indicates the type of swap.
	Type swap.Type

	// Channels is the set of channels that a loop out swap is using.
	Channels []lnwire.ShortChannelID

	// Peer is the last hop set for loop in (if any).
	Peer *route.Vertex
}

// NewExistingSwap creates an existing swap with information about the channels
// and peers the swap is restricted to, if any.
func NewExistingSwap(hash lntypes.Hash, state loopdb.SwapState,
	swapType swap.Type, channels []lnwire.ShortChannelID,
	peer *route.Vertex) ExistingSwap {

	return ExistingSwap{
		SwapHash: hash,
		State:    state,
		Type:     swapType,
		Channels: channels,
		Peer:     peer,
	}
}

// Manager contains a set of desired liquidity rules for our channel
// balances.
type Manager struct {
	// cfg contains the external functionality we require to determine our
	// current liquidity balance.
	cfg *Config

	// params is the set of parameters we are currently using. These may be
	// updated at runtime.
	params Parameters

	// paramsLock is a lock for our current set of parameters.
	paramsLock sync.Mutex
}

// NewManager creates a liquidity manager which has no rules set.
func NewManager(cfg *Config) *Manager {
	return &Manager{
		cfg:    cfg,
		params: newParameters(),
	}
}

// GetParameters returns a copy of our current parameters.
func (m *Manager) GetParameters() Parameters {
	m.paramsLock.Lock()
	defer m.paramsLock.Unlock()

	return cloneParameters(m.params)
}

// SetParameters updates our current set of parameters if the new parameters
// provided are valid.
func (m *Manager) SetParameters(params Parameters) error {
	if err := params.validate(); err != nil {
		return err
	}

	m.paramsLock.Lock()
	defer m.paramsLock.Unlock()

	m.params = cloneParameters(params)
	return nil
}

// cloneParameters creates a deep clone of a parameters struct so that callers
// cannot mutate our parameters. Although our parameters struct itself is not
// a reference, we still need to clone the contents of maps.
func cloneParameters(params Parameters) Parameters {
	paramCopy := Parameters{
		ChannelRules: make(map[lnwire.ShortChannelID]*ThresholdRule,
			len(params.ChannelRules)),
	}

	for channel, rule := range params.ChannelRules {
		ruleCopy := *rule
		paramCopy.ChannelRules[channel] = &ruleCopy
	}

	return paramCopy
}

// SuggestSwaps returns a set of swap suggestions based on our current liquidity
// balance for the set of rules configured for the manager, failing if there are
// no rules set.
func (m *Manager) SuggestSwaps(ctx context.Context) (
	[]*LoopOutRecommendation, error) {

	m.paramsLock.Lock()
	defer m.paramsLock.Unlock()

	// If we have no rules set, exit early to avoid unnecessary calls to
	// lnd and the server.
	if len(m.params.ChannelRules) == 0 {
		return nil, nil
	}

	// Get the current server side restrictions.
	outRestrictions, err := m.cfg.LoopOutRestrictions(ctx)
	if err != nil {
		return nil, err
	}

	// List our current set of swaps so that we can determine which channels
	// are already being utilized by swaps.
	allSwaps, err := m.cfg.ListSwaps(ctx)
	if err != nil {
		return nil, err
	}

	eligible, err := m.getEligibleChannels(ctx, allSwaps)
	if err != nil {
		return nil, err
	}

	var suggestions []*LoopOutRecommendation
	for _, channel := range eligible {
		channelID := lnwire.NewShortChanIDFromInt(channel.ChannelID)
		rule, ok := m.params.ChannelRules[channelID]
		if !ok {
			continue
		}

		balance := newBalances(channel)

		suggestion := rule.suggestSwap(balance, outRestrictions)

		// We can have nil suggestions in the case where no action is
		// required, so only add non-nil suggestions.
		if suggestion != nil {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions, nil
}

// getEligibleChannels takes a set of existing swaps, gets a list of channels
// that are not currently being utilized for a swap which we can suggest swaps
// for. If an unrestricted swap is ongoing, we return an empty set of channels
// because we don't know which channels balances it will affect.
func (m *Manager) getEligibleChannels(ctx context.Context,
	allSwaps []ExistingSwap) ([]lndclient.ChannelInfo, error) {

	var (
		existingOut = make(map[lnwire.ShortChannelID]bool)
		existingIn  = make(map[route.Vertex]bool)
	)

	for _, s := range allSwaps {
		// We can ignore swaps that are not in a pending state, because
		// they will not be affecting our current set of channel
		// balances going forward, they are resolved.
		if s.State.Type() != loopdb.StateTypePending {
			continue
		}

		// If our swap is un-restricted, return early because we cannot
		// suggest swaps when we are uncertain where these currently
		// ongoing swaps will shift our balance. If we have a limit on
		// the swap's off chain path, we add it to our set of unusable
		// peers or channels.
		switch s.Type {
		case swap.TypeIn:
			if s.Peer == nil {
				log.Infof("ongoing unrestricted loop in: "+
					"%v, no suggestions at present",
					s.SwapHash)

				return nil, nil
			}

			existingIn[*s.Peer] = true

		case swap.TypeOut:
			if len(s.Channels) == 0 {
				log.Infof("ongoing unrestricted loop out: "+
					"%v, no suggestions at present",
					s.SwapHash)

				return nil, nil
			}

			for _, channel := range s.Channels {
				existingOut[channel] = true
			}

		default:
			return nil, fmt.Errorf("unknown swap type: %v", s.Type)
		}

	}

	channels, err := m.cfg.Lnd.ListChannels(ctx)
	if err != nil {
		return nil, err
	}

	// Run through our set of channels and skip over any channels that
	// are currently being utilized by a restricted swap (where restricted
	// means that a loop out limited channels, or a loop in limited last
	// hop).
	var eligible []lndclient.ChannelInfo
	for _, channel := range channels {
		shortID := lnwire.NewShortChanIDFromInt(channel.ChannelID)

		if existingOut[shortID] {
			log.Infof("channel: %v not eligible for "+
				"suggestions, ongoing loop out utilizing "+
				"channel", channel.ChannelID)

			continue
		}

		if existingIn[channel.PubKeyBytes] {
			log.Infof("channel: %v not eligible for "+
				"suggestions, ongoing loop in utilizing "+
				"peer", channel.ChannelID)

			continue
		}

		eligible = append(eligible, channel)
	}

	return eligible, nil
}
