package liquidity

import (
	"context"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/labels"
	"github.com/lightningnetwork/lnd/routing/route"
)

type loopInBuilder struct {
	params Parameters
	cfg    *Config
}

func newLoopInBuilder(params Parameters, cfg *Config) *loopInBuilder {
	return &loopInBuilder{
		params: params,
		cfg:    cfg,
	}
}

func (b *loopInBuilder) createSuggestion(ctx context.Context,
	amount btcutil.Amount, balance *balances, autoloop bool) (
	*loop.LoopInRequest, Reason, error) {

	// TODO(carla): add HtlcConfTarget
	quote, err := b.cfg.LoopInQuote(ctx, &loop.LoopInQuoteRequest{})
	if err != nil {
		return nil, 0, err
	}

	log.Debugf("quote for suggestion: %v, swap fee: %v, miner fee: %v, "+
		"cltv delta: %v", quote.SwapFee, quote.MinerFee,
		quote.CltvDelta)

	// TODO(carla): add checks for each of the quote things

	inRequest := b.makeLoopInRequest(
		ctx, amount, balance.pubkey, quote, autoloop,
	)

	return &inRequest, ReasonNone, nil
}

// makeLoopInRequest creates a request for a loop in swap for the amount and
// peer provided. It uses the quote provided for fee limitations, assuming that
// these values have already been checked against our configured limits.
func (b *loopInBuilder) makeLoopInRequest(ctx context.Context,
	amount btcutil.Amount, peer route.Vertex, quote *loop.LoopInQuote,
	autoloop bool) loop.LoopInRequest {

	resp := loop.LoopInRequest{
		Amount:      amount,
		MaxSwapFee:  quote.SwapFee,
		MaxMinerFee: quote.MinerFee,
		LastHop:     &peer,
		Initiator:   autoloopSwapInitiator,
	}

	if autoloop {
		resp.Label = labels.AutoloopLabel(false)
	}

	return resp
}
