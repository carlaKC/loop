// Package liquidity is responsible for monitoring our node's liquidity.
package liquidity

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/btcsuite/btcutil"
)

var (
	// ErrNoParameters is returned when a request is made to lookup manager
	// parameters, but none are set.
	ErrNoParameters = errors.New("no parameters set for manager")

	// ErrSingleRule is returned when parameters try to set liquidity rules
	// per-node and per-peer at the same time. These rules could contradict
	// each other, so setting both is not allowed.
	ErrSingleRule = errors.New("liquidity rules can be set on a per " +
		"node or per peer basis, not both")

	// ErrShuttingDown is returned when a request is cancelled because
	// the manager is shutting down.
	ErrShuttingDown = errors.New("server shutting down")
)

// Config contains the external functionality required to run the liquidity
// manager.
type Config struct {
	// ServerInRestrictions are the limits placed on loop in by the server.
	ServerInRestrictions Restrictions

	// ServerOutRestrictions are the limits placed on loop out by the
	// server.
	ServerOutRestrictions Restrictions
}

// Parameters is a set of parameters provided by the user which guide how we
// assess liquidity.
type Parameters struct {
	// NodeRule is a rule that is applied to our liquidity on a per-node
	// level. This option is used to set a universal liquidity rule for a
	// node, and cannot be set if PeerRule is set.
	NodeRule Rule

	// PeerRule is a liquidity rule that is applied to our liquidity on a
	// per-peer level. This option is used to set liquidity rules that apply
	// to all of the channels we have with a peer, and cannot be set if
	// NodeRule is set.
	PeerRule Rule

	// IncludePrivate indicates whether we should include private channels
	// in our balance calculations.
	IncludePrivate bool
}

// String returns the string representation of our parameters.
func (p *Parameters) String() string {
	return fmt.Sprintf("include private: %v, node rule: %v, "+
		"peer rule: %v", p.IncludePrivate, p.NodeRule, p.PeerRule)
}

// validate checks whether a set of parameters is valid.
func (p *Parameters) validate() error {
	nodeRuleSet := p.NodeRule != nil
	peerRuleSet := p.PeerRule != nil

	// Fail if both generalized rules are set, we do not want this because
	// it could lead to rules which contradict one another.
	if nodeRuleSet && peerRuleSet {
		return ErrSingleRule
	}

	if nodeRuleSet {
		if err := p.NodeRule.validate(); err != nil {
			return err
		}
	}

	if peerRuleSet {
		if err := p.PeerRule.validate(); err != nil {
			return err
		}
	}

	return nil
}

// Restrictions describe the restrictions placed on swaps.
type Restrictions struct {
	// MinimumAmount is the lower limit on swap amount, inclusive.
	MinimumAmount btcutil.Amount

	// MaximumAmount is the upper limit on swap amount, inclusive.
	MaximumAmount btcutil.Amount
}

// NewRestrictions creates a new set of restrictions.
func NewRestrictions(minimum, maximum btcutil.Amount) Restrictions {
	return Restrictions{
		MinimumAmount: minimum,
		MaximumAmount: maximum,
	}
}

// String returns the string representation of our restriction.
func (r Restrictions) String() string {
	return fmt.Sprintf("%v-%v", r.MinimumAmount, r.MaximumAmount)
}

// Manager monitors our ratios of incoming and outgoing liquidity, recommending
// loops based on the required ratios configured.
type Manager struct {
	started int32 // to be used atomically

	// cfg contains the external functionality we require to determine our
	// current liquidity balance.
	cfg *Config

	// params is the set of parameters we are currently using. These may be
	// updated at runtime.
	params *Parameters

	// paramRequests is a channel that requests to update our current set
	// of parameters are sent on.
	paramRequests chan updateParamsRequest

	// done is closed when our main event loop is shutting down. This allows
	// us to cancel requests sent to our main event loop that cannot be
	// served.
	done chan struct{}
}

// updateParamsRequest contains a set of updates to apply to our current config
// and a channel to send our response on. If nil parameters are provided, this
// request serves as a lookup.
type updateParamsRequest struct {
	params   *Parameters
	response chan *Parameters
}

// NewManager creates a liquidity manager which has no parameters set.
func NewManager(cfg *Config) *Manager {
	return &Manager{
		cfg:           cfg,
		params:        nil,
		done:          make(chan struct{}),
		paramRequests: make(chan updateParamsRequest),
	}
}

// Run starts the manager, failing if it has already been started. Note that
// this function will block, so should be run in a goroutine.
func (m *Manager) Run(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.started, 0, 1) {
		return errors.New("manager already started")
	}

	return m.run(ctx)
}

// run is the main event loop for our liquidity manager. When it exits, it
// closes the done channel so that any pending requests sent into our request
// channel can be cancelled.
func (m *Manager) run(ctx context.Context) error {
	defer close(m.done)

	for {
		select {
		// Serve requests to update or view our current parameters.
		case request := <-m.paramRequests:
			// If the parameters we were sent are non-nil, we
			// update our parameters to the already-validated set
			// of parameters.
			if request.params != nil {
				m.params = request.params
				log.Info("updated parameters: %v", m.params)
			}

			// Send our current parameters into the response
			// channel.
			request.response <- m.params

		// Return a non-nil error if we receive the instruction to exit.
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// UpdateParameters delivers a request to update our parameters if it is
// provided with a non-nil set of parameters, and requests a copy of our current
// parameters if provided with nil parameters. This function handles making
// copies of the pointers provided and returned so that mutation by the caller
// will not affect our internal copy of the parameters. If no parameters are
// currently set, this function will return ErrNoParameters.
func (m *Manager) UpdateParameters(ctx context.Context,
	params *Parameters) (*Parameters, error) {

	// If the parameters passed in are non-nil, we check that the proposed
	// parameters are valid, then make a copy that we will pass to the main
	// event loop.
	var requestParameters *Parameters
	if params != nil {
		if err := params.validate(); err != nil {
			return nil, err
		}

		paramCopy := *params
		requestParameters = &paramCopy
	}

	// Send a request to our main event loop to process the updates,
	// buffering the response channel so that the event loop cannot be
	// blocked by the client not consuming the request.
	responseChan := make(chan *Parameters, 1)
	select {
	case m.paramRequests <- updateParamsRequest{
		params:   requestParameters,
		response: responseChan,
	}:

	case <-m.done:
		return nil, ErrShuttingDown

	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for a response from the main event loop, or client cancellation.
	select {
	// If the loop response with nil parameters, return ErrNoParameters
	// because none are available. Otherwise, make a copy and return it.
	case newParams := <-responseChan:
		if newParams == nil {
			return nil, ErrNoParameters
		}

		newCopy := *newParams
		return &newCopy, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
