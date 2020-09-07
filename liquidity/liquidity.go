// Package liquidity is responsible for monitoring our node's liquidity. It
// allows setting of a liquidity rule which describes the desired liquidity
// balance on a per-channel basis.
package liquidity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

const (
	// DefaultFailureBackOff is the default amount of time we wait after
	// a swap has failed on a channel to recommend using it again.
	DefaultFailureBackOff = time.Hour * 24 * 7

	// DefaultSwapFeePPM is the default limit we place of the fee paid to
	// the server, expressed as parts per million of the swap volume, equal
	// to 0.5 of a percent.
	DefaultSwapFeePPM = 5000

	// DefaultPrepay is the default limit we place on prepayments.
	DefaultPrepay btcutil.Amount = 20000

	// DefaultMinerFee is the default limit we place on miner fees.
	DefaultMinerFee btcutil.Amount = 15000

	// DefaultConfTarget is the default confirmation target we use for htlc
	// sweeps.
	// TODO(carla): refactor and use loop.DefaultSweepConfTarget
	DefaultConfTarget = 9

	// FeeBase is the base that we use to express fees.
	FeeBase = 1000000
)

var (
	// ErrZeroChannelID is returned if we get a rule for a 0 channel ID.
	ErrZeroChannelID = fmt.Errorf("zero channel ID not allowed")

	// ErrZeroMinerFee is returned if a zero maximum miner fee is set.
	ErrZeroMinerFee = errors.New("maximum miner fee must be non-zero")

	// ErrZeroSwapFeePPM is returned if a zero server fee ppm is set.
	ErrZeroSwapFeePPM = errors.New("swap fee PPM must be non-zero")

	// ErrZeroPrepay is returned if a zero maximum prepay is set.
	ErrZeroPrepay = errors.New("maximum prepay must be non-zero")

	// ErrConfTargetTooLow is returned if a conf target that is lower than
	// our backing fee estimator can allow is set.
	ErrConfTargetTooLow = errors.New("conf target must be at least 2")

	errInsufficientMinerFee = errors.New("miner fee above maximum")

	errInsufficientPrepay = errors.New("prepay above maximum")

	errInsufficientSwapFee = errors.New("swap fee above maximum")
)

// Config contains the external functionality required to run the
// liquidity manager.
type Config struct {
	// LoopOutRestrictions returns the restrictions that the server applies
	// to loop out swaps.
	LoopOutRestrictions func(ctx context.Context) (*Restrictions, error)

	// LoopOutQuote gets swap fee, estimated miner fee and prepay amount for
	// a loop out swap.
	LoopOutQuote func(ctx context.Context, amount btcutil.Amount,
		confTarget int32) (btcutil.Amount, btcutil.Amount,
		btcutil.Amount, error)

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
	// FailureBackOff is the amount of time that we require passes after a
	// channel has been part of a failed loop out swap before we suggest
	// using it again.
	FailureBackOff time.Duration

	// MaximumPrepay is the maximum prepay amount we are willing to pay per
	// swap.
	MaximumPrepay btcutil.Amount

	// MaximumSwapFeePPM is the maximum server fee we are willing to pay per
	// swap expressed as parts per million of the swap volume.
	MaximumSwapFeePPM int

	// MaximumMinerFee is the maximum on chain fee we are willing to pay.
	MaximumMinerFee btcutil.Amount

	// ConfTarget is the number of blocks we aim to confirm our sweep
	// transaction in. This value affects our miner fees.
	ConfTarget int32

	// ChannelRules maps a short channel ID to a rule that describes how we
	// would like liquidity to be managed.
	ChannelRules map[lnwire.ShortChannelID]*ThresholdRule
}

// newParameters creates an empty set of parameters.
func newParameters() Parameters {
	return Parameters{
		FailureBackOff:    DefaultFailureBackOff,
		MaximumSwapFeePPM: DefaultSwapFeePPM,
		MaximumPrepay:     DefaultPrepay,
		MaximumMinerFee:   DefaultMinerFee,
		ConfTarget:        DefaultConfTarget,
		ChannelRules:      make(map[lnwire.ShortChannelID]*ThresholdRule),
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

	// Check that we have non-zero fee limits.
	if p.MaximumSwapFeePPM == 0 {
		return ErrZeroSwapFeePPM
	}

	if p.MaximumPrepay == 0 {
		return ErrZeroPrepay
	}

	if p.MaximumMinerFee == 0 {
		return ErrZeroMinerFee
	}

	// Check that our confirmation target is above our required minimum.
	// TODO(carla): refactor and use rpc validation
	if p.ConfTarget < 2 {
		return ErrConfTargetTooLow
	}

	return nil
}

// ExistingSwap provides information about a swap that has been dispatched.
type ExistingSwap struct {
	// LastUpdate is the timestamp of the last update applied to the swap.
	// If the swap has no updates, this value will be its created time.
	LastUpdate time.Time

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
func NewExistingSwap(lastUpdate time.Time, hash lntypes.Hash,
	state loopdb.SwapState, swapType swap.Type,
	channels []lnwire.ShortChannelID, peer *route.Vertex) ExistingSwap {

	return ExistingSwap{
		LastUpdate: lastUpdate,
		SwapHash:   hash,
		State:      state,
		Type:       swapType,
		Channels:   channels,
		Peer:       peer,
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
		FailureBackOff:    params.FailureBackOff,
		MaximumPrepay:     params.MaximumPrepay,
		MaximumMinerFee:   params.MaximumMinerFee,
		MaximumSwapFeePPM: params.MaximumSwapFeePPM,
		ConfTarget:        params.ConfTarget,
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
		// required, so we skip over them.
		if suggestion == nil {
			continue
		}

		// Get a quote for a swap of this amount.
		swapFee, minerFee, prepay, err := m.cfg.LoopOutQuote(
			ctx, suggestion.Amount, m.params.ConfTarget,
		)
		if err != nil {
			return nil, err
		}

		log.Infof("quote for suggestion: %v, swap fee: %v, "+
			"miner fee: %v, prepay:%v", suggestion, swapFee,
			minerFee, prepay)

		// Check that the estimated fees for the suggested swap are
		// below the fee limits configured by the manager.
		err = m.checkFeeLimits(swapFee, minerFee, prepay, suggestion)
		if err != nil {
			log.Infof("suggestion: %v expected fees too high: %v",
				suggestion, err)

			continue
		}

		suggestions = append(suggestions, suggestion)
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
		failedOut   = make(map[lnwire.ShortChannelID]time.Time)
	)

	// Failure cutoff is the most recent failure timestamp we will still
	// consider a channel eligible. Any channels involved in swaps that have
	// failed since this point will not be considered.
	failureCutoff := m.cfg.Clock.Now().Add(m.params.FailureBackOff * -1)

	for _, s := range allSwaps {
		// If a loop out swap failed due to off chain payment after our
		// failure cutoff, we add all of its channels to a set of
		// recently failed channels. It is possible that not all of
		// these channels were used for the swap, but we play it safe
		// and back off for all of them.
		if s.State == loopdb.StateFailOffchainPayments &&
			s.Type == swap.TypeOut {

			if s.LastUpdate.After(failureCutoff) {
				for _, channel := range s.Channels {
					failedOut[channel] = s.LastUpdate
				}
			}
		}

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

		lastFail, recentFail := failedOut[shortID]
		if recentFail {
			log.Infof("channel: %v not eligible for "+
				"suggestions, was part of a failed swap at: %v",
				channel.ChannelID, lastFail)

			continue
		}

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

// checkFeeLimits takes a set of fees for a swap and checks whether they exceed
// our swap limits.
func (m *Manager) checkFeeLimits(swap, miner, prepay btcutil.Amount,
	suggestion *LoopOutRecommendation) error {

	allowedFee := ppmToSat(suggestion.Amount, m.params.MaximumSwapFeePPM)

	if swap > allowedFee {
		return errInsufficientSwapFee
	}

	if miner > m.params.MaximumMinerFee {
		return errInsufficientMinerFee
	}

	if prepay > m.params.MaximumPrepay {
		return errInsufficientPrepay
	}

	return nil
}

// ppmToSat takes an amount and a measure of parts per million for the amount
// and returns the amount that the ppm represents.
func ppmToSat(amount btcutil.Amount, ppm int) btcutil.Amount {
	return btcutil.Amount(uint64(amount) * uint64(ppm) / FeeBase)
}
