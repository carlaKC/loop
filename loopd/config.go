package loopd

import (
	"path/filepath"

	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/loop/lsat"
)

var (
	loopDirBase = btcutil.AppDataDir("loop", false)

	defaultLogLevel    = "info"
	defaultLogDirname  = "logs"
	defaultLogFilename = "loopd.log"
	defaultLogDir      = filepath.Join(loopDirBase, defaultLogDirname)

	defaultMaxLogFiles     = 3
	defaultMaxLogFileSize  = 10
	defaultLoopOutMaxParts = uint32(5)
	defaultHtlcConfs       = uint32(1)
)

type lndConfig struct {
	Host        string `long:"host" description:"lnd instance rpc address"`
	MacaroonDir string `long:"macaroondir" description:"Path to the directory containing all the required lnd macaroons"`
	TLSPath     string `long:"tlspath" description:"Path to lnd tls certificate"`
}

type viewParameters struct{}

type Config struct {
	ShowVersion    bool   `long:"version" description:"Display version information and exit"`
	Insecure       bool   `long:"insecure" description:"disable tls"`
	Network        string `long:"network" description:"network to run on" choice:"regtest" choice:"testnet" choice:"mainnet" choice:"simnet"`
	SwapServer     string `long:"swapserver" description:"swap server address host:port"`
	TLSPathSwapSrv string `long:"tlspathswapserver" description:"Path to swap server tls certificate. Only needed if the swap server uses a self-signed certificate."`
	RPCListen      string `long:"rpclisten" description:"Address to listen on for gRPC clients"`
	RESTListen     string `long:"restlisten" description:"Address to listen on for REST clients"`
	CORSOrigin     string `long:"corsorigin" description:"The value to send in the Access-Control-Allow-Origin header. Header will be omitted if empty."`

	LogDir         string `long:"logdir" description:"Directory to log output."`
	MaxLogFiles    int    `long:"maxlogfiles" description:"Maximum logfiles to keep (0 for no rotation)"`
	MaxLogFileSize int    `long:"maxlogfilesize" description:"Maximum logfile size in MB"`

	DebugLevel  string `long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`
	MaxLSATCost uint32 `long:"maxlsatcost" description:"Maximum cost in satoshis that loopd is going to pay for an LSAT token automatically. Does not include routing fees."`
	MaxLSATFee  uint32 `long:"maxlsatfee" description:"Maximum routing fee in satoshis that we are willing to pay while paying for an LSAT token."`

	LoopOutMaxParts uint32 `long:"loopoutmaxparts" description:"The maximum number of payment parts that may be used for a loop out swap."`

	HtlcConfirmations uint32 `long:"htlcconfs" description:"Confirmation target for on chain htlcs (blocks)."`

	Lnd   *lndConfig `group:"lnd" namespace:"lnd"`
	Proxy string     `long:"proxy" description:"The host:port of a SOCKS proxy through which all connections to the swap server will be established over."`

	View viewParameters `command:"view" alias:"v" description:"View all swaps in the database. This command can only be executed when loopd is not running."`
}

const (
	mainnetServer = "swap.lightning.today:11010"
	testnetServer = "test.swap.lightning.today:11010"
)

// DefaultConfig returns all default values for the Config struct.
func DefaultConfig() Config {
	return Config{
		Network:           "mainnet",
		RPCListen:         "localhost:11010",
		RESTListen:        "localhost:8081",
		Insecure:          false,
		LogDir:            defaultLogDir,
		MaxLogFiles:       defaultMaxLogFiles,
		MaxLogFileSize:    defaultMaxLogFileSize,
		DebugLevel:        defaultLogLevel,
		MaxLSATCost:       lsat.DefaultMaxCostSats,
		MaxLSATFee:        lsat.DefaultMaxRoutingFeeSats,
		LoopOutMaxParts:   defaultLoopOutMaxParts,
		HtlcConfirmations: defaultHtlcConfs,
		Lnd: &lndConfig{
			Host: "localhost:10009",
		},
	}
}
