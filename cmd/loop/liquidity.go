package main

import (
	"context"
	"fmt"

	"github.com/lightninglabs/loop/looprpc"
	"github.com/urfave/cli"
)

var (
	nodeTarget = "node"
	peerTarget = "peer"
)

var getLiquidityCfgCommand = cli.Command{
	Name:  "getcfg",
	Usage: "show liquidity manager parameters",
	Description: "Displays the current set of parameters and rules that " +
		"are set for the liquidity manager subsystem.",
	Action: getCfg,
}

func getCfg(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	cfg, err := client.GetLiquidityConfig(
		context.Background(), &looprpc.GetLiquidityConfigRequest{},
	)
	if err != nil {
		return err
	}

	printJSON(cfg)

	return nil
}

var setLiquidityCfgCommand = cli.Command{
	Name:  "setcfg",
	Usage: "set liquidity manager parameters",
	Description: "Updates the current set of parameters that are set " +
		"for the liquidity manager subsystem.",
	Flags: []cli.Flag{
		cli.Float64Flag{
			Name: "mininbound",
			Usage: "the minimum ratio of inbound liquidity to " +
				"total capacity beneath which to recommend " +
				"loop out to acquire inbound.",
		},
		cli.Float64Flag{
			Name: "minoutbound",
			Usage: "the minimum ratio of outbound liquidity to" +
				"total capacity beneath which to recommend " +
				"loop in to acquire outbound.",
		},
		cli.BoolFlag{
			Name: "inclprivate",
			Usage: "whether to include private channels in our " +
				"balance calculations.",
		},
	},
	Action: setCfg,
}

func setCfg(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// We need to set all values in the config that we send the server to
	// update us to. To allow users to set only individual fields on the
	// cli, we lookup our current config, then update its values.
	cfg, err := client.GetLiquidityConfig(
		context.Background(), &looprpc.GetLiquidityConfigRequest{},
	)
	if err != nil {
		return err
	}

	// If our config was not set before this, we set it to a non-nil value
	// now.
	if cfg == nil {
		cfg = &looprpc.LiquidityConfig{}
	}

	if ctx.IsSet("inclprivate") {
		cfg.IncludePrivate = ctx.Bool("inclprivate")
	}

	// Create a request to update our config.
	req := &looprpc.SetLiquidityConfigRequest{
		Config: cfg,
	}

	cfg, err = client.SetLiquidityConfig(context.Background(), req)
	if err != nil {
		return err
	}

	printJSON(cfg)

	return nil
}

var setLiquidityRuleCommand = cli.Command{
	Name:  "setrule",
	Usage: "set liquidity manger rule for a target",
	Description: "Updates the liquidity rule that we set for a target. " +
		"At present rules can be set for your node as a whole, or on " +
		"a per-peer level.",
	ArgsUsage: "[node|peer]",
	Flags: []cli.Flag{
		cli.Float64Flag{
			Name: "mininbound",
			Usage: "the minimum ratio of inbound liquidity to " +
				"total capacity beneath which to recommend " +
				"loop out to acquire inbound.",
		},
		cli.Float64Flag{
			Name: "minoutbound",
			Usage: "the minimum ratio of outbound liquidity to" +
				"total capacity beneath which to recommend " +
				"loop in to acquire outbound.",
		},
		cli.BoolFlag{
			Name:  "clear",
			Usage: "remove the rule for the current target.",
		},
	},
	Action: setRule,
}

func setRule(ctx *cli.Context) error {
	// We require that a target is set for this rule update.
	if ctx.NArg() != 1 {
		return fmt.Errorf("please set a target for the rule "+
			"update: %v or %v", nodeTarget, peerTarget)
	}

	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// We need to set all values in the config that we send the server to
	// update us to. To allow users to set only individual fields on the
	// cli, we lookup our current config, then update its values.
	cfg, err := client.GetLiquidityConfig(
		context.Background(), &looprpc.GetLiquidityConfigRequest{},
	)
	if err != nil {
		return err
	}

	// updateRule is a helper function which updates our set of rules to
	// the set provided.
	updateRule := func(cfg *looprpc.LiquidityConfig) error {
		cfg, err := client.SetLiquidityConfig(
			context.Background(),
			&looprpc.SetLiquidityConfigRequest{Config: cfg},
		)
		if err != nil {
			return err
		}

		printJSON(cfg)
		return nil
	}

	// We either want to set our node or peer rule, depending on our target,
	// but leave the remainder of the config intact. We use this function
	// to malleate the config appropriately.
	var setRule func(rule *looprpc.LiquidityRule)

	target := ctx.Args().First()

	switch target {
	case nodeTarget:
		setRule = func(rule *looprpc.LiquidityRule) {
			cfg.NodeRule = rule
		}

	case peerTarget:
		setRule = func(rule *looprpc.LiquidityRule) {
			cfg.PeerRule = rule
		}

	default:
		return fmt.Errorf("unknown rule target: %v", target)
	}

	// If the clear flag is set, we clear the rule and exit early.
	if ctx.IsSet("clear") {
		setRule(nil)
		return updateRule(cfg)
	}

	// Create a new rule which will be used to overwrite our current rule.
	newRule := &looprpc.LiquidityRule{}

	if ctx.IsSet("mininbound") {
		newRule.MinimumInbound = float32(ctx.Float64("mininbound"))
		newRule.Type = looprpc.LiquidityRuleType_RATIO
	}

	if ctx.IsSet("minoutbound") {
		newRule.MinimumOutbound = float32(ctx.Float64("minoutbound"))
		newRule.Type = looprpc.LiquidityRuleType_RATIO
	}

	if newRule.Type == looprpc.LiquidityRuleType_UNKNOWN {
		return fmt.Errorf("please update at least one parameter or " +
			"use the clear flag to remove the target's rule")
	}

	setRule(newRule)
	return updateRule(cfg)
}

var suggestSwapCommand = cli.Command{
	Name:  "suggestswaps",
	Usage: "show a list of suggested swaps",
	Description: "Displays a list of suggested swaps that aim to obtain " +
		"the liquidity thresholds set out in the autolooper config. ",
	Action: suggestSwap,
}

func suggestSwap(ctx *cli.Context) error {
	client, cleanup, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	resp, err := client.SuggestSwaps(
		context.Background(), &looprpc.SuggestSwapsRequest{},
	)
	if err != nil {
		return err
	}

	printJSON(resp)

	return nil
}
