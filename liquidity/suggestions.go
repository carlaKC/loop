package liquidity

import (
	"fmt"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightningnetwork/lnd/lnwire"
)

// LoopOutRecommendation contains the information required to recommend a loop
// out.
type LoopOutRecommendation struct {
	// Amount is the total amount to swap.
	Amount btcutil.Amount

	// Channels is the set of channels that the swap is restricted to.
	Channels loopdb.ChannelSet
}

// String returns a string representation of a loop out recommendation.
func (l *LoopOutRecommendation) String() string {
	return fmt.Sprintf("loop out: %v over %v", l.Amount, l.Channels)
}

// newLoopOutRecommendation creates a new loop out swap.
func newLoopOutRecommendation(amount btcutil.Amount,
	channels []lnwire.ShortChannelID) *LoopOutRecommendation {

	var chanSet loopdb.ChannelSet
	for _, channel := range channels {
		chanSet = append(chanSet, channel.ToUint64())
	}

	return &LoopOutRecommendation{
		Amount:   amount,
		Channels: chanSet,
	}
}
