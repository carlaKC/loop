package liquidity

import (
	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/lnwire"
)

// balances summarizes the state of the balances of a channel. Channel reserve,
// fees and pending htlc balances are not included in these balances.
type balances struct {
	// capacity is the total capacity of the channel.
	capacity btcutil.Amount

	// incoming is the remote balance of the channel.
	incoming btcutil.Amount

	// outgoing is the local balance of the channel.
	outgoing btcutil.Amount

	// channels is the channel that has these balances represent. This may
	// be more than one channel in the case where we are examining a peer's
	// liquidity as a whole.
	channels []lnwire.ShortChannelID
}

// newBalances creates a balances struct from lndclient channel information.
func newBalances(info lndclient.ChannelInfo) *balances {
	return &balances{
		capacity: info.Capacity,
		incoming: info.RemoteBalance,
		outgoing: info.LocalBalance,
		channels: []lnwire.ShortChannelID{
			lnwire.NewShortChanIDFromInt(info.ChannelID),
		},
	}
}
