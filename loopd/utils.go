package loopd

import (
	"context"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/liquidity"
	"github.com/lightningnetwork/lnd/clock"
)

// getClient returns an instance of the swap client.
func getClient(config *Config, lnd *lndclient.LndServices) (*loop.Client,
	func(), error) {

	clientConfig := &loop.ClientConfig{
		ServerAddress:   config.Server.Host,
		ProxyAddress:    config.Server.Proxy,
		SwapServerNoTLS: config.Server.NoTLS,
		TLSPathServer:   config.Server.TLSPath,
		Lnd:             lnd,
		MaxLsatCost:     btcutil.Amount(config.MaxLSATCost),
		MaxLsatFee:      btcutil.Amount(config.MaxLSATFee),
		LoopOutMaxParts: config.LoopOutMaxParts,
	}

	swapClient, cleanUp, err := loop.NewClient(config.DataDir, clientConfig)
	if err != nil {
		return nil, nil, err
	}

	return swapClient, cleanUp, nil
}

func getLiquidityManager(client *loop.Client) *liquidity.Manager {
	mngrCfg := &liquidity.Config{
		LoopOutRestrictions: func(ctx context.Context) (
			*liquidity.Restrictions, error) {

			outTerms, err := client.Server.GetLoopOutTerms(ctx)
			if err != nil {
				return nil, err
			}

			return liquidity.NewRestrictions(
				outTerms.MinSwapAmount, outTerms.MaxSwapAmount,
			), nil
		},
		Lnd:   client.LndServices.Client,
		Clock: clock.NewDefaultClock(),
		LoopOutQuote: func(ctx context.Context, amount btcutil.Amount,
			confTarget int32) (btcutil.Amount, btcutil.Amount,
			btcutil.Amount, error) {

			quote, err := client.LoopOutQuote(
				ctx, &loop.LoopOutQuoteRequest{
					Amount:          amount,
					SweepConfTarget: confTarget,
				},
			)
			if err != nil {
				return 0, 0, 0, err
			}

			return quote.SwapFee, quote.MinerFee,
				quote.PrepayAmount, nil
		},
		ListSwaps: func(ctx context.Context) (
			[]liquidity.ExistingSwap, error) {

			swaps, err := client.FetchSwaps()
			if err != nil {
				return nil, err
			}

			existingSwaps := make(
				[]liquidity.ExistingSwap, len(swaps),
			)

			for i, swap := range swaps {
				existingSwaps[i] = liquidity.NewExistingSwap(
					swap.LastUpdate, swap.SwapHash,
					swap.State, swap.SwapType,
					swap.OutgoingChannels, swap.LastHop,
				)
			}

			return existingSwaps, nil
		},
	}

	return liquidity.NewManager(mngrCfg)
}
