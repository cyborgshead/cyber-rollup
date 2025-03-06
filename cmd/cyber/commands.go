package cyber

import (
	"errors"
	"fmt"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/version"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/zeta-chain/ethermint/server/config"
	srvflags "github.com/zeta-chain/node/server/flags"

	//etherminttypes "github.com/zeta-chain/ethermint/types"
	types "github.com/cyborgshead/cyber-rollup/app/params"
	"github.com/zeta-chain/node/pkg/cosmos"

	//zetaservercfg "github.com/zeta-chain/node/server/config"
	"io"
	"os"

	cmtcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cmtcfg "github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"cosmossdk.io/log"
	confixcmd "cosmossdk.io/tools/confix/cmd"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"

	"github.com/CosmWasm/wasmd/x/wasm"
	wasmcli "github.com/CosmWasm/wasmd/x/wasm/client/cli"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cyborgshead/cyber-rollup/app"

	mainserver "github.com/cyborgshead/cyber-rollup/server"
	rollconf "github.com/rollkit/rollkit/config"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethermintclient "github.com/zeta-chain/ethermint/client"

	cosmosserver "github.com/cosmos/cosmos-sdk/server"
	cosmosservertypes "github.com/cosmos/cosmos-sdk/server/types"
	zetacrypto "github.com/zeta-chain/node/pkg/crypto"
	zetamempool "github.com/zeta-chain/node/pkg/mempool"
	zetaserver "github.com/zeta-chain/node/server"
	zetaconfig "github.com/zeta-chain/node/server/config"
)

var KeyAddCommand = []string{"keys", "add"}

const (
	HDPathFlag     = "hd-path"
	HDPathEthereum = "m/44'/60'/0'/0/0"
)

// SetEthereumHDPath sets the default HD path to Ethereum's
func SetEthereumHDPath(cmd *cobra.Command) error {
	return ReplaceFlag(cmd, KeyAddCommand, HDPathFlag, HDPathEthereum)
}

// ReplaceFlag replaces the default value of a flag of a sub-command
func ReplaceFlag(cmd *cobra.Command, subCommand []string, flagName, newDefaultValue string) error {
	// Find the sub-command
	c, _, err := cmd.Find(subCommand)
	if err != nil {
		return fmt.Errorf("failed to find %v sub-command: %v", subCommand, err)
	}

	// Get the flag from the sub-command
	f := c.Flags().Lookup(flagName)
	if f == nil {
		return fmt.Errorf("%s flag not found in %v sub-command", flagName, subCommand)
	}

	// Set the default value for the flag
	f.DefValue = newDefaultValue
	if err := f.Value.Set(newDefaultValue); err != nil {
		return fmt.Errorf("failed to set the value of %s flag: %v", flagName, err)
	}

	return nil
}

// initCometBFTConfig helps to override default CometBFT Config values.
// return cmtcfg.DefaultConfig if no custom configuration is required for the application.
func initCometBFTConfig() *cmtcfg.Config {
	cfg := cmtcfg.DefaultConfig()

	// these values put a higher strain on node memory
	// cfg.P2P.MaxNumInboundPeers = 100
	// cfg.P2P.MaxNumOutboundPeers = 40

	return cfg
}

// initAppConfig helps to override default appConfig template and configs.
// return "", nil if no custom configuration is required for the application.
func initAppConfig() (string, interface{}) {
	// The following code snippet is just for reference.

	type CustomAppConfig struct {
		serverconfig.Config

		EVM     zetaconfig.EVMConfig     `mapstructure:"evm"`
		JSONRPC zetaconfig.JSONRPCConfig `mapstructure:"json-rpc"`
		TLS     zetaconfig.TLSConfig     `mapstructure:"tls"`

		Wasm wasmtypes.NodeConfig `mapstructure:"wasm"`
	}

	// Optionally allow the chain developer to overwrite the SDK's default
	// server config.
	srvCfg := serverconfig.DefaultConfig()
	// The SDK's default minimum gas price is set to "" (empty value) inside
	// app.toml. If left empty by validators, the node will halt on startup.
	// However, the chain developer can set a default app.toml value for their
	// validators here.
	//
	// In summary:
	// - if you leave srvCfg.MinGasPrices = "", all validators MUST tweak their
	//   own app.toml config,
	// - if you set srvCfg.MinGasPrices non-empty, validators CAN tweak their
	//   own app.toml to override, or use this default value.
	//
	// In simapp, we set the min gas prices to 0.
	srvCfg.MinGasPrices = "0stake"
	// srvCfg.BaseConfig.IAVLDisableFastNode = true // disable fastnode by default

	customAppConfig := CustomAppConfig{
		Config:  *srvCfg,
		Wasm:    wasmtypes.DefaultNodeConfig(),
		EVM:     *zetaconfig.DefaultEVMConfig(),
		JSONRPC: *zetaconfig.DefaultJSONRPCConfig(),
		TLS:     *zetaconfig.DefaultTLSConfig(),
	}

	customAppTemplate := serverconfig.DefaultConfigTemplate +
		wasmtypes.DefaultConfigTemplate() + zetaconfig.DefaultConfigTemplate

	return customAppTemplate, customAppConfig
}

func initRootCmd(
	rootCmd *cobra.Command,
	encodingConfig types.EncodingConfig,
	basicManager module.BasicManager,
) {

	ac := appCreator{
		encCfg: encodingConfig,
	}

	cfg := sdk.GetConfig()
	cfg.Seal()

	rootCmd.AddCommand(
		ethermintclient.ValidateChainID(
			genutilcli.InitCmd(basicManager, app.DefaultNodeHome),
		),
		genutilcli.InitCmd(basicManager, app.DefaultNodeHome),
		genutilcli.CollectGenTxsCmd(
			banktypes.GenesisBalancesIterator{},
			app.DefaultNodeHome,
			genutiltypes.DefaultMessageValidator,
			encodingConfig.TxConfig.SigningContext().ValidatorAddressCodec(),
		),
		genutilcli.GenTxCmd(
			basicManager,
			encodingConfig.TxConfig,
			banktypes.GenesisBalancesIterator{},
			app.DefaultNodeHome,
			encodingConfig.TxConfig.SigningContext().ValidatorAddressCodec(),
		),
		genutilcli.ValidateGenesisCmd(basicManager),
		confixcmd.ConfigCommand(),
		pruning.Cmd(ac.newApp, app.DefaultNodeHome),

		NewTestnetCmd(basicManager, banktypes.GenesisBalancesIterator{}),
		GetPubKeyCmd(),
		AddrConversionCmd(),

		ethermintclient.NewTestnetCmd(basicManager, banktypes.GenesisBalancesIterator{}),

		debug.Cmd(),
		snapshot.Cmd(ac.newApp),
	)

	//server.AddCommandsWithStartCmdOptions(
	//	rootCmd,
	//	app.DefaultNodeHome,
	//	ac.newApp, appExport,
	//	server.StartCmdOptions{
	//		AddFlags:            rollconf.AddFlags,
	//		StartCommandHandler: mainserver.StartHandler[servertypes.Application],
	//	},
	//)
	//server.AddCommandsWithStartCmdOptions(
	AddCommandsWithStartCmdOptions(
		rootCmd,
		app.DefaultNodeHome,
		ac.newApp, appExport,
		addModuleInitFlags,
		server.StartCmdOptions{
			AddFlags: func(cmd *cobra.Command) {
				rollconf.AddFlags(cmd)
				AddEthermintFlags(cmd)
			},
			StartCommandHandler: mainserver.StartHandler[servertypes.Application],
		},
	)
	//server.AddCommands(rootCmd, app.DefaultNodeHome, newApp, appExport, addModuleInitFlags)
	wasmcli.ExtendUnsafeResetAllCmd(rootCmd)

	// add keybase, auxiliary RPC, query, genesis, and tx child commands
	rootCmd.AddCommand(
		server.StatusCommand(),
		genesisCommand(encodingConfig.TxConfig, basicManager),
		queryCommand(),
		txCommand(),
		keys.Commands(),
		ethermintclient.KeyCommands(app.DefaultNodeHome),
	)

	// TODO replace the default hd-path for the key add command with Ethereum HD Path
	if err := SetEthereumHDPath(rootCmd); err != nil {
		fmt.Printf("warning: unable to set default HD path: %v\n", err)
	}
}

func AddCommandsWithStartCmdOptions(rootCmd *cobra.Command, defaultNodeHome string, appCreator cosmosservertypes.AppCreator, appExport cosmosservertypes.AppExporter, addStartFlags cosmosservertypes.ModuleInitFlags, opts cosmosserver.StartCmdOptions) {
	cometCmd := &cobra.Command{
		Use:     "comet",
		Aliases: []string{"cometbft", "tendermint"},
		Short:   "CometBFT subcommands",
	}

	cometCmd.AddCommand(
		cosmosserver.ShowNodeIDCmd(),
		cosmosserver.ShowValidatorCmd(),
		cosmosserver.ShowAddressCmd(),
		cosmosserver.VersionCmd(),
		cmtcmd.ResetAllCmd,
		cmtcmd.ResetStateCmd,
		cosmosserver.BootstrapStateCmd(appCreator),
	)

	startCmd := cosmosserver.StartCmdWithOptions(appCreator, defaultNodeHome, opts)
	addStartFlags(startCmd)

	rootCmd.AddCommand(
		startCmd,
		cometCmd,
		cosmosserver.ExportCmd(appExport, defaultNodeHome),
		version.NewVersionCommand(),
		cosmosserver.NewRollbackCmd(appCreator, defaultNodeHome),

		// custom tx indexer command
		zetaserver.NewIndexTxCmd(),
	)
}

func AddEthermintFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(srvflags.JSONRPCEnable, true, "Define if the JSON-RPC server should be enabled")
	cmd.Flags().
		StringSlice(srvflags.JSONRPCAPI, config.GetDefaultAPINamespaces(), "Defines a list of JSON-RPC namespaces that should be enabled")
	cmd.Flags().
		String(srvflags.JSONRPCAddress, config.DefaultJSONRPCAddress, "the JSON-RPC server address to listen on")
	cmd.Flags().
		String(srvflags.JSONWsAddress, config.DefaultJSONRPCWsAddress, "the JSON-RPC WS server address to listen on")
	cmd.Flags().
		Uint64(srvflags.JSONRPCGasCap, config.DefaultGasCap, "Sets a cap on gas that can be used in eth_call/estimateGas unit is aphoton (0=infinite)")

	//nolint:lll
	cmd.Flags().
		Float64(srvflags.JSONRPCTxFeeCap, config.DefaultTxFeeCap, "Sets a cap on transaction fee that can be sent via the RPC APIs (1 = default 1 photon)")

	//nolint:lll
	cmd.Flags().
		Int32(srvflags.JSONRPCFilterCap, config.DefaultFilterCap, "Sets the global cap for total number of filters that can be created")
	cmd.Flags().
		Duration(srvflags.JSONRPCEVMTimeout, config.DefaultEVMTimeout, "Sets a timeout used for eth_call (0=infinite)")
	cmd.Flags().
		Duration(srvflags.JSONRPCHTTPTimeout, config.DefaultHTTPTimeout, "Sets a read/write timeout for json-rpc http server (0=infinite)")
	cmd.Flags().
		Duration(srvflags.JSONRPCHTTPIdleTimeout, config.DefaultHTTPIdleTimeout, "Sets a idle timeout for json-rpc http server (0=infinite)")
	cmd.Flags().
		Bool(srvflags.JSONRPCAllowUnprotectedTxs, config.DefaultAllowUnprotectedTxs, "Allow for unprotected (non EIP155 signed) transactions to be submitted via the node's RPC when the global parameter is disabled")

	//nolint:lll
	cmd.Flags().
		Int32(srvflags.JSONRPCLogsCap, config.DefaultLogsCap, "Sets the max number of results can be returned from single `eth_getLogs` query")
	cmd.Flags().
		Int32(srvflags.JSONRPCBlockRangeCap, config.DefaultBlockRangeCap, "Sets the max block range allowed for `eth_getLogs` query")
	cmd.Flags().
		Int(srvflags.JSONRPCMaxOpenConnections, config.DefaultMaxOpenConnections, "Sets the maximum number of simultaneous connections for the server listener")

	//nolint:lll
	cmd.Flags().Bool(srvflags.JSONRPCEnableIndexer, false, "Enable the custom tx indexer for json-rpc")
	cmd.Flags().Bool(srvflags.JSONRPCEnableMetrics, false, "Define if EVM rpc metrics server should be enabled")

	cmd.Flags().
		String(srvflags.EVMTracer, config.DefaultEVMTracer, "the EVM tracer type to collect execution traces from the EVM transaction execution (json|struct|access_list|markdown)")

	//nolint:lll
	cmd.Flags().
		Uint64(srvflags.EVMMaxTxGasWanted, config.DefaultMaxTxGasWanted, "the gas wanted for each eth tx returned in ante handler in check tx mode")

	//nolint:lll

	cmd.Flags().String(srvflags.TLSCertPath, "", "the cert.pem file path for the server TLS configuration")
	cmd.Flags().String(srvflags.TLSKeyPath, "", "the key.pem file path for the server TLS configuration")
}
func addModuleInitFlags(startCmd *cobra.Command) {
	crisis.AddModuleInitFlags(startCmd)
	wasm.AddModuleInitFlags(startCmd)
}

// genesisCommand builds genesis-related `simd genesis` command. Users may provide application specific commands as a parameter
func genesisCommand(txConfig client.TxConfig, basicManager module.BasicManager, cmds ...*cobra.Command) *cobra.Command {
	cmd := genutilcli.Commands(txConfig, basicManager, app.DefaultNodeHome)

	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}
	return cmd
}

func queryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.ValidatorCommand(),
		rpc.QueryEventForTxCmd(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.QueryTxCmd(),
		server.QueryBlockCmd(),
		server.QueryBlocksCmd(),
		server.QueryBlockResultsCmd(),
	)

	return cmd
}

func txCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
	)

	return cmd
}

type appCreator struct {
	encCfg types.EncodingConfig
}

// newApp creates the application
func (ac appCreator) newApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	baseappOptions := server.DefaultBaseappOptions(appOpts)

	var wasmOpts []wasmkeeper.Option
	if cast.ToBool(appOpts.Get("telemetry.enabled")) {
		wasmOpts = append(wasmOpts, wasmkeeper.WithVMCacheMetrics(prometheus.DefaultRegisterer))
	}

	maxTxs := cast.ToInt(appOpts.Get(server.FlagMempoolMaxTxs))
	if maxTxs <= 0 {
		maxTxs = zetamempool.DefaultMaxTxs
	}
	baseappOptions = append(baseappOptions, func(app *baseapp.BaseApp) {
		app.SetMempool(zetamempool.NewPriorityMempool(zetamempool.PriorityNonceWithMaxTx(maxTxs)))
	})
	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	return app.NewCyberApp(
		logger, db, traceStore, true,
		appOpts,
		wasmOpts,
		baseappOptions...,
	)
}

// appExport creates a new wasm app (optionally at a given height) and exports state.
func appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var cyberApp *app.CyberApp
	// this check is necessary as we use the flag in x/upgrade.
	// we can exit more gracefully by checking the flag here.
	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home is not set")
	}

	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}

	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(server.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	var emptyWasmOpts []wasmkeeper.Option
	cyberApp = app.NewCyberApp(
		logger,
		db,
		traceStore,
		height == -1,
		appOpts,
		emptyWasmOpts,
	)

	if height != -1 {
		if err := cyberApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	}

	return cyberApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}

var tempDir = func() string {
	dir, err := os.MkdirTemp("", "cyber")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}

	return dir
}

func GetPubKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-pubkey [tssKeyName] [password]",
		Short: "Get the node account public key",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			tssKeyName := args[0]
			password := ""
			if len(args) > 1 {
				password = args[1]
			}
			pubKeySet, err := GetPubKeySet(clientCtx, tssKeyName, password)
			if err != nil {
				return err
			}
			fmt.Println(pubKeySet.String())
			return nil
		},
	}
	return cmd
}

func GetPubKeySet(clientctx client.Context, tssAccountName, password string) (zetacrypto.PubKeySet, error) {
	pubkeySet := zetacrypto.PubKeySet{
		Secp256k1: "",
		Ed25519:   "",
	}
	//kb, err := GetKeyringKeybase(keyringPath, tssAccountName, password)
	privKeyArmor, err := clientctx.Keyring.ExportPrivKeyArmor(tssAccountName, password)
	if err != nil {
		return pubkeySet, err
	}
	priKey, _, err := crypto.UnarmorDecryptPrivKey(privKeyArmor, password)
	if err != nil {
		return pubkeySet, fmt.Errorf("fail to unarmor private key: %w", err)
	}

	s, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, priKey.PubKey())
	if err != nil {
		return pubkeySet, err
	}
	pubkey, err := zetacrypto.NewPubKey(s)
	if err != nil {
		return pubkeySet, err
	}
	pubkeySet.Secp256k1 = pubkey
	return pubkeySet, nil
}

func AddrConversionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addr-conversion [zeta address]",
		Short: "convert a zeta1xxx address to validator operator address zetavaloper1xxx",
		Long: `
read a zeta1xxx or zetavaloper1xxx address and convert it to the other type;
it always outputs three lines; the first line is the zeta1xxx address, the second line is the zetavaloper1xxx address
and the third line is the ethereum address.
			`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			addr, err := sdk.AccAddressFromBech32(args[0])
			if err == nil {
				valAddr := sdk.ValAddress(addr.Bytes())
				fmt.Printf("%s\n", addr.String())
				fmt.Printf("%s\n", valAddr.String())
				fmt.Printf("%s\n", ethcommon.BytesToAddress(addr.Bytes()).String())
				return nil
			}
			valAddr, err := sdk.ValAddressFromBech32(args[0])
			if err == nil {
				addr := sdk.AccAddress(valAddr.Bytes())
				fmt.Printf("%s\n", addr.String())
				fmt.Printf("%s\n", valAddr.String())
				fmt.Printf("%s\n", ethcommon.BytesToAddress(addr.Bytes()).String())
				return nil
			}
			ethAddr := ethcommon.HexToAddress(args[0])
			if ethAddr != (ethcommon.Address{}) {
				addr := sdk.AccAddress(ethAddr.Bytes())
				valAddr := sdk.ValAddress(addr.Bytes())
				fmt.Printf("%s\n", addr.String())
				fmt.Printf("%s\n", valAddr.String())
				fmt.Printf("%s\n", ethAddr.String())
				return nil
			}
			return fmt.Errorf("invalid address: %s", args[0])
		},
	}
	return cmd
}
