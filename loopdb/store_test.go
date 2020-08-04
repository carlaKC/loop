package loopdb

import (
	"crypto/sha256"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/coreos/bbolt"
	"github.com/lightninglabs/loop/test"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

var (
	senderKey = [33]byte{
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2,
	}

	receiverKey = [33]byte{
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3,
	}

	testPreimage = lntypes.Preimage([32]byte{
		1, 1, 1, 1, 2, 2, 2, 2,
		3, 3, 3, 3, 4, 4, 4, 4,
		1, 1, 1, 1, 2, 2, 2, 2,
		3, 3, 3, 3, 4, 4, 4, 4,
	})

	testTime = time.Date(2018, time.January, 9, 14, 00, 00, 0, time.UTC)
)

// TestLoopOutStore tests all the basic functionality of the current bbolt
// swap store.
func TestLoopOutStore(t *testing.T) {
	destAddr := test.GetDestAddr(t, 0)
	initiationTime := time.Date(2018, 11, 1, 0, 0, 0, 0, time.UTC)

	// Next, we'll make a new pending swap that we'll insert into the
	// database shortly.
	unrestrictedSwap := LoopOutContract{
		SwapContract: SwapContract{
			AmountRequested: 100,
			Preimage:        testPreimage,
			CltvExpiry:      144,
			SenderKey:       senderKey,

			ReceiverKey: receiverKey,
			MaxMinerFee: 10,
			MaxSwapFee:  20,

			InitiationHeight: 99,

			// Convert to/from unix to remove timezone, so that it
			// doesn't interfere with DeepEqual.
			InitiationTime: time.Unix(0, initiationTime.UnixNano()),
		},
		MaxPrepayRoutingFee:     40,
		PrepayInvoice:           "prepayinvoice",
		DestAddr:                destAddr,
		SwapInvoice:             "swapinvoice",
		MaxSwapRoutingFee:       30,
		SweepConfTarget:         2,
		HtlcConfirmations:       2,
		SwapPublicationDeadline: time.Unix(0, initiationTime.UnixNano()),
	}

	t.Run("no outgoing set", func(t *testing.T) {
		testLoopOutStore(t, &unrestrictedSwap)
	})

	restrictedSwap := unrestrictedSwap
	restrictedSwap.OutgoingChanSet = ChannelSet{1, 2}

	t.Run("two channel outgoing set", func(t *testing.T) {
		testLoopOutStore(t, &restrictedSwap)
	})
}

// testLoopOutStore tests the basic functionality of the current bbolt
// swap store for specific swap parameters.
func testLoopOutStore(t *testing.T, pendingSwap *LoopOutContract) {
	tempDirName, err := ioutil.TempDir("", "clientstore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)

	store, err := NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	// First, verify that an empty database has no active swaps.
	swaps, err := store.FetchLoopOutSwaps()
	if err != nil {
		t.Fatal(err)
	}
	if len(swaps) != 0 {
		t.Fatal("expected empty store")
	}

	// checkSwap is a test helper function that'll assert the state of a
	// swap.
	checkSwap := func(expectedState SwapState) {
		t.Helper()

		swaps, err := store.FetchLoopOutSwaps()
		if err != nil {
			t.Fatal(err)
		}

		if len(swaps) != 1 {
			t.Fatal("expected pending swap in store")
		}

		swap := swaps[0].Contract
		if !reflect.DeepEqual(swap, pendingSwap) {
			t.Fatal("invalid pending swap data")
		}

		if swaps[0].State().State != expectedState {
			t.Fatalf("expected state %v, but got %v",
				expectedState, swaps[0].State(),
			)
		}

		if expectedState == StatePreimageRevealed {
			require.NotNil(t, swaps[0].State().HtlcTxHash)
		}
	}

	hash := pendingSwap.Preimage.Hash()

	// If we create a new swap, then it should show up as being initialized
	// right after.
	if err := store.CreateLoopOut(hash, pendingSwap); err != nil {
		t.Fatal(err)
	}
	checkSwap(StateInitiated)

	// Trying to make the same swap again should result in an error.
	if err := store.CreateLoopOut(hash, pendingSwap); err == nil {
		t.Fatal("expected error on storing duplicate")
	}
	checkSwap(StateInitiated)

	// Next, we'll update to the next state of the pre-image being
	// revealed. The state should be reflected here again.
	err = store.UpdateLoopOut(
		hash, testTime,
		SwapStateData{
			State:      StatePreimageRevealed,
			HtlcTxHash: &chainhash.Hash{1, 6, 2},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StatePreimageRevealed)

	// Next, we'll update to the final state to ensure that the state is
	// properly updated.
	err = store.UpdateLoopOut(
		hash, testTime,
		SwapStateData{
			State: StateFailInsufficientValue,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StateFailInsufficientValue)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// If we re-open the same store, then the state of the current swap
	// should be the same.
	store, err = NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StateFailInsufficientValue)
}

// TestLoopInStore tests all the basic functionality of the current bbolt
// swap store.
func TestLoopInStore(t *testing.T) {
	tempDirName, err := ioutil.TempDir("", "clientstore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)

	store, err := NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	// First, verify that an empty database has no active swaps.
	swaps, err := store.FetchLoopInSwaps()
	if err != nil {
		t.Fatal(err)
	}
	if len(swaps) != 0 {
		t.Fatal("expected empty store")
	}

	hash := sha256.Sum256(testPreimage[:])
	initiationTime := time.Date(2018, 11, 1, 0, 0, 0, 0, time.UTC)

	// Next, we'll make a new pending swap that we'll insert into the
	// database shortly.
	lastHop := route.Vertex{1, 2, 3}

	pendingSwap := LoopInContract{
		SwapContract: SwapContract{
			AmountRequested:  100,
			Preimage:         testPreimage,
			CltvExpiry:       144,
			SenderKey:        senderKey,
			ReceiverKey:      receiverKey,
			MaxMinerFee:      10,
			MaxSwapFee:       20,
			InitiationHeight: 99,

			// Convert to/from unix to remove timezone, so that it
			// doesn't interfere with DeepEqual.
			InitiationTime: time.Unix(0, initiationTime.UnixNano()),
		},
		HtlcConfTarget: 2,
		LastHop:        &lastHop,
		ExternalHtlc:   true,
	}

	// checkSwap is a test helper function that'll assert the state of a
	// swap.
	checkSwap := func(expectedState SwapState) {
		t.Helper()

		swaps, err := store.FetchLoopInSwaps()
		if err != nil {
			t.Fatal(err)
		}

		if len(swaps) != 1 {
			t.Fatal("expected pending swap in store")
		}

		swap := swaps[0].Contract
		if !reflect.DeepEqual(swap, &pendingSwap) {
			t.Fatal("invalid pending swap data")
		}

		if swaps[0].State().State != expectedState {
			t.Fatalf("expected state %v, but got %v",
				expectedState, swaps[0].State(),
			)
		}
	}

	// If we create a new swap, then it should show up as being initialized
	// right after.
	if err := store.CreateLoopIn(hash, &pendingSwap); err != nil {
		t.Fatal(err)
	}
	checkSwap(StateInitiated)

	// Trying to make the same swap again should result in an error.
	if err := store.CreateLoopIn(hash, &pendingSwap); err == nil {
		t.Fatal("expected error on storing duplicate")
	}
	checkSwap(StateInitiated)

	// Next, we'll update to the next state of the pre-image being
	// revealed. The state should be reflected here again.
	err = store.UpdateLoopIn(
		hash, testTime,
		SwapStateData{
			State: StatePreimageRevealed,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StatePreimageRevealed)

	// Next, we'll update to the final state to ensure that the state is
	// properly updated.
	err = store.UpdateLoopIn(
		hash, testTime,
		SwapStateData{
			State: StateFailInsufficientValue,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StateFailInsufficientValue)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// If we re-open the same store, then the state of the current swap
	// should be the same.
	store, err = NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	checkSwap(StateFailInsufficientValue)
}

// TestVersionNew tests that a new database is initialized with the current
// version.
func TestVersionNew(t *testing.T) {
	tempDirName, err := ioutil.TempDir("", "clientstore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)

	store, err := NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	ver, err := getDBVersion(store.db)
	if err != nil {
		t.Fatal(err)
	}

	if ver != latestDBVersion {
		t.Fatal("db not at latest version")
	}
}

// TestVersionNew tests that an existing version zero database is migrated to
// the latest version.
func TestVersionMigrated(t *testing.T) {
	tempDirName, err := ioutil.TempDir("", "clientstore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)

	createVersionZeroDb(t, tempDirName)

	store, err := NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	ver, err := getDBVersion(store.db)
	if err != nil {
		t.Fatal(err)
	}

	if ver != latestDBVersion {
		t.Fatal("db not at latest version")
	}
}

// createVersionZeroDb creates a database with an empty meta bucket. In version
// zero, there was no version key specified yet.
func createVersionZeroDb(t *testing.T, dbPath string) {
	path := filepath.Join(dbPath, dbFileName)
	bdb, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bdb.Close()

	err = bdb.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucket(metaBucketKey)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLegacyOutgoingChannel asserts that a legacy channel restriction is
// properly mapped onto the newer channel set.
func TestLegacyOutgoingChannel(t *testing.T) {
	var (
		legacyDbVersion       = Hex("00000003")
		legacyOutgoingChannel = Hex("0000000000000005")
	)

	legacyDb := map[string]interface{}{
		"loop-in": map[string]interface{}{},
		"metadata": map[string]interface{}{
			"dbp": legacyDbVersion,
		},
		"uncharge-swaps": map[string]interface{}{
			Hex("2a595d79a55168970532805ae20c9b5fac98f04db79ba4c6ae9b9ac0f206359e"): map[string]interface{}{
				"contract": Hex("1562d6fbec140000010101010202020203030303040404040101010102020202030303030404040400000000000000640d707265706179696e766f69636501010101010101010101010101010101010101010101010101010101010101010201010101010101010101010101010101010101010101010101010101010101010300000090000000000000000a0000000000000014000000000000002800000063223347454e556d6e4552745766516374344e65676f6d557171745a757a5947507742530b73776170696e766f69636500000002000000000000001e") + legacyOutgoingChannel + Hex("1562d6fbec140000"),
				"updates": map[string]interface{}{
					Hex("0000000000000001"): Hex("1508290a92d4c00001000000000000000000000000000000000000000000000000"),
					Hex("0000000000000002"): Hex("1508290a92d4c00006000000000000000000000000000000000000000000000000"),
				},
			},
		},
	}

	// Restore a legacy database.
	tempDirName, err := ioutil.TempDir("", "clientstore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)

	tempPath := filepath.Join(tempDirName, dbFileName)
	db, err := bbolt.Open(tempPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		return RestoreDB(tx, legacyDb)
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Fetch the legacy swap.
	store, err := NewBoltSwapStore(tempDirName, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	swaps, err := store.FetchLoopOutSwaps()
	if err != nil {
		t.Fatal(err)
	}

	// Assert that the outgoing channel is read properly.
	expectedChannelSet := ChannelSet{5}
	if !reflect.DeepEqual(swaps[0].Contract.OutgoingChanSet, expectedChannelSet) {
		t.Fatal("invalid outgoing channel")
	}
}
