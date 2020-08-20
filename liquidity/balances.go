package liquidity

import (
	"github.com/btcsuite/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
)

// balances summarizes the state of the balances on our node. Channel reserve,
// fees and pending htlc balances are not included in these balances.
type balances struct {
	// capacity is the total capacity in all of our channels.
	capacity btcutil.Amount

	// incoming is the total remote balance across all channels.
	incoming btcutil.Amount

	// outgoing is the total local balance across all channels.
	outgoing btcutil.Amount

	// channelID is short channel id of channel that has this set of
	// balances.
	channelID lnwire.ShortChannelID
}

// incomingRatio returns our ratio of incoming to total capacity.
func (b *balances) incomingRatio() float32 {
	return float32(b.incoming) / float32(b.capacity)
}

// outgoingRatio returns our ratio of outgoing to total capacity.
func (b *balances) outgoingRatio() float32 {
	return float32(b.outgoing) / float32(b.capacity)
}
