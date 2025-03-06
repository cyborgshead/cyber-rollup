package server

import (
	"context"
	errorsmod "cosmossdk.io/errors"
	"fmt"
	"github.com/cometbft/cometbft/node"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/zeta-chain/ethermint/indexer"
	srvflags "github.com/zeta-chain/node/server/flags"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servercmtlog "github.com/cosmos/cosmos-sdk/server/log"
	sdktypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/cosmos/cosmos-sdk/version"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/hashicorp/go-metrics"
	ethermint "github.com/zeta-chain/ethermint/types"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rollkitconfig "github.com/rollkit/rollkit/config"
	rollkitnode "github.com/rollkit/rollkit/node"
	//rollkitrpc "github.com/rollkit/rollkit/rpc"
	rollkittypes "github.com/rollkit/rollkit/types"

	zetaos "github.com/zeta-chain/node/pkg/os"
	zetaserver "github.com/zeta-chain/node/server"
	zetaconfig "github.com/zeta-chain/node/server/config"

	// EVM related imports
	"github.com/cometbft/cometbft/rpc/client/local"
	ethmetricsexp "github.com/ethereum/go-ethereum/metrics/exp"
	//"github.com/cyborgshead/cyber-rollup/server/flags"
	cyberapp "github.com/cyborgshead/cyber-rollup/app"
)

const (
	flagTraceStore = "trace-store"
	flagTransport  = "transport"
	flagGRPCOnly   = "grpc-only"
)

func startApp(svrCtx *server.Context, appCreator sdktypes.AppCreator, opts server.StartCmdOptions) (app sdktypes.Application, cleanupFn func(), err error) {
	traceWriter, traceCleanupFn, err := setupTraceWriter(svrCtx)
	if err != nil {
		return app, traceCleanupFn, err
	}

	home := svrCtx.Config.RootDir
	db, err := opts.DBOpener(home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return app, traceCleanupFn, err
	}

	app = appCreator(svrCtx.Logger, db, traceWriter, svrCtx.Viper)

	cleanupFn = func() {
		traceCleanupFn()
		if localErr := app.Close(); localErr != nil {
			svrCtx.Logger.Error(localErr.Error())
		}
	}
	return app, cleanupFn, nil
}

// StartHandler starts the Rollkit server with the provided application and options.
func StartHandler[T sdktypes.Application](svrCtx *server.Context, clientCtx client.Context, appCreator sdktypes.AppCreator, inProcess bool, opts server.StartCmdOptions) error {
	svrCfg, err := getAndValidateConfig(svrCtx)
	if err != nil {
		return err
	}

	//app, appCleanupFn, err := startApp(svrCtx, appCreator, opts)
	//if err != nil {
	//	return err
	//}
	//defer appCleanupFn()

	metrics, err := startTelemetry(svrCfg)
	if err != nil {
		return err
	}

	emitServerInfoMetrics()

	//return startInProcess[T](svrCtx, svrCfg, clientCtx, app, metrics, opts)
	return startInProcess(svrCtx, svrCfg, clientCtx, metrics, zetaserver.NewDefaultStartOptions(appCreator, cyberapp.DefaultNodeHome))
}

// func startInProcess[T sdktypes.Application](
// svrCtx *server.Context,
// svrCfg serverconfig.Config,
// clientCtx client.Context,
// app sdktypes.Application,
// metrics *telemetry.Metrics,
// opts server.StartCmdOptions,
func startInProcess(ctx *server.Context, svrCfg serverconfig.Config, clientCtx client.Context, metrics *telemetry.Metrics, opts zetaserver.StartOptions) (err error) {
	cfg := ctx.Config
	home := cfg.RootDir
	logger := ctx.Logger
	g, c := getCtx(ctx, true)

	if cpuProfile := ctx.Viper.GetString(srvflags.CPUProfile); cpuProfile != "" {
		fp, err := zetaos.ExpandHomeDir(cpuProfile)
		if err != nil {
			ctx.Logger.Debug("failed to get filepath for the CPU profile file", "error", err.Error())
			return err
		}
		// #nosec G304 - users can't control the filepath
		f, err := os.Create(fp)
		if err != nil {
			return err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		defer func() {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				logger.Error("failed to close CPU profiler file", "error", err.Error())
			}
		}()
	}

	db, err := opts.DBOpener(ctx.Viper, home, server.GetAppDBBackend(ctx.Viper))
	if err != nil {
		logger.Error("failed to open DB", "error", err.Error())
		return err
	}

	defer func() {
		if err := db.Close(); err != nil {
			ctx.Logger.With("error", err).Error("error closing db")
		}
	}()

	//traceWriterFile := ctx.Viper.GetString(srvflags.TraceStore)
	//traceWriter, err := openTraceWriter(traceWriterFile)
	//if err != nil {
	//	logger.Error("failed to open trace writer", "error", err.Error())
	//	return err
	//}

	config, err := zetaconfig.GetConfig(ctx.Viper)
	if err != nil {
		logger.Error("failed to get server config", "error", err.Error())
		return err
	}

	if err := config.ValidateBasic(); err != nil {
		logger.Error("invalid server config", "error", err.Error())
		return err
	}

	traceWriterFile := ctx.Viper.GetString(srvflags.TraceStore)
	traceWriter, err := openTraceWriter(traceWriterFile)
	app := opts.AppCreator(ctx.Logger, db, traceWriter, ctx.Viper)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		logger.Error("failed load or gen node key", "error", err.Error())
		return err
	}

	genDocProvider := GenDocProvider(cfg)

	var (
		tmNode   *node.Node
		gRPCOnly = ctx.Viper.GetBool(srvflags.GRPCOnly)
	)

	if gRPCOnly {
		logger.Info("starting node in query only mode; Tendermint is disabled")
		config.GRPC.Enable = true
		config.JSONRPC.EnableIndexer = false
	} else {
		logger.Info("starting node with ABCI Tendermint in-process")

		cmtApp := server.NewCometABCIWrapper(app)

		logger.Info("starting node with Rollkit in-process")

		pval := pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())

		//keys in Rollkit format
		p2pKey, err := rollkittypes.GetNodeKey(nodeKey)
		if err != nil {
			return err
		}

		signingKey, err := rollkittypes.GetNodeKey(&p2p.NodeKey{PrivKey: pval.Key.PrivKey})
		if err != nil {
			return err
		}

		nodeConfig := rollkitconfig.NodeConfig{}
		err = nodeConfig.GetViperConfig(ctx.Viper)
		if err != nil {
			return err
		}
		rollkitconfig.GetNodeConfig(&nodeConfig, cfg)
		err = rollkitconfig.TranslateAddresses(&nodeConfig)
		if err != nil {
			return err
		}

		genDoc, err := getGenDocProvider(cfg)()
		if err != nil {
			return err
		}

		tmNode, err := rollkitnode.NewNode(
			c,
			nodeConfig,
			p2pKey,
			signingKey,
			proxy.NewLocalClientCreator(cmtApp),
			genDoc,
			rollkitnode.DefaultMetricsProvider(cfg.Instrumentation),
			servercmtlog.CometLoggerWrapper{Logger: ctx.Logger.With("server", "node")},
		)

		//tmNode, err = node.NewNodeWithContext(
		//	c,
		//	cfg,
		//	pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
		//	nodeKey,
		//	proxy.NewLocalClientCreator(cmtApp),
		//	genDocProvider,
		//	cmtcfg.DefaultDBProvider,
		//	node.DefaultMetricsProvider(cfg.Instrumentation),
		//	servercmtlog.CometLoggerWrapper{Logger: ctx.Logger.With("server", "node")},
		//)

		if err != nil {
			logger.Error("failed init node", "error", err.Error())
			return err
		}

		if err := tmNode.Start(); err != nil {
			logger.Error("failed start tendermint server", "error", err.Error())
			return err
		}

		defer func() {
			if tmNode.IsRunning() {
				err = tmNode.Stop()
			}
		}()
	}

	// Add the tx service to the gRPC router. We only need to register this
	// service if API or gRPC or JSONRPC is enabled, and avoid doing so in the general
	// case, because it spawns a new local tendermint RPC client.
	if (config.API.Enable || config.GRPC.Enable || config.JSONRPC.Enable || config.JSONRPC.EnableIndexer) &&
		tmNode != nil {
		clientCtx = clientCtx.WithClient(local.New(tmNode))

		app.RegisterTxService(clientCtx)
		app.RegisterTendermintService(clientCtx)
		app.RegisterNodeService(clientCtx, config.Config)
	}

	// Enable metrics if JSONRPC is enabled and --metrics is passed
	// Flag not added in config to avoid user enabling in config without passing in CLI
	if config.JSONRPC.Enable && ctx.Viper.GetBool(srvflags.JSONRPCEnableMetrics) {
		ethmetricsexp.Setup(config.JSONRPC.MetricsAddress)
	}

	var idxer ethermint.EVMTxIndexer
	if config.JSONRPC.EnableIndexer {
		idxDB, err := OpenIndexerDB(home, server.GetAppDBBackend(ctx.Viper))
		if err != nil {
			logger.Error("failed to open evm indexer DB", "error", err.Error())
			return err
		}

		idxLogger := ctx.Logger.With("indexer", "evm")
		idxer = indexer.NewKVIndexer(idxDB, idxLogger, clientCtx)
		indexerService := zetaserver.NewEVMIndexerService(idxer, clientCtx.Client.(rpcclient.Client))
		indexerService.SetLogger(servercmtlog.CometLoggerWrapper{Logger: idxLogger})

		go func() {
			if err := indexerService.Start(); err != nil {
				logger.Error("failed to start evm indexer service", "error", err.Error())
			}
		}()
	}

	if config.API.Enable || config.JSONRPC.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx = clientCtx.
			WithHomeDir(home).
			WithChainID(genDoc.ChainID)

		// Set `GRPCClient` to `clientCtx` to enjoy concurrent grpc query.
		// only use it if gRPC server is enabled.
		if config.GRPC.Enable {
			_, port, err := net.SplitHostPort(config.GRPC.Address)
			if err != nil {
				return errorsmod.Wrapf(err, "invalid grpc address %s", config.GRPC.Address)
			}

			maxSendMsgSize := config.GRPC.MaxSendMsgSize
			if maxSendMsgSize == 0 {
				maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
			}

			maxRecvMsgSize := config.GRPC.MaxRecvMsgSize
			if maxRecvMsgSize == 0 {
				maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
			}

			grpcAddress := fmt.Sprintf("127.0.0.1:%s", port)

			// If grpc is enabled, configure grpc client for grpc gateway and json-rpc.
			grpcClient, err := grpc.Dial(
				grpcAddress,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithDefaultCallOptions(
					grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
					grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
					grpc.MaxCallSendMsgSize(maxSendMsgSize),
				),
			)
			if err != nil {
				return err
			}

			clientCtx = clientCtx.WithGRPCClient(grpcClient)
			ctx.Logger.Debug("gRPC client assigned to client context", "address", grpcAddress)
		}
	}

	grpcSrv, clientCtx, err := startGrpcServer(c, ctx, clientCtx, g, config.GRPC, app)
	if err != nil {
		return err
	}

	var apiSrv *api.Server
	if config.API.Enable {
		apiSrv = api.New(clientCtx, ctx.Logger.With("server", "api"), grpcSrv)
		app.RegisterAPIRoutes(apiSrv, config.API)

		if config.Telemetry.Enabled {
			apiSrv.SetTelemetry(metrics)
		}

		g.Go(func() error {
			return apiSrv.Start(c, config.Config)
		})
	}

	var (
		httpSrv     *http.Server
		httpSrvDone chan struct{}
	)

	if config.JSONRPC.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx := clientCtx.WithChainID(genDoc.ChainID)

		tmEndpoint := "/websocket"
		tmRPCAddr := cfg.RPC.ListenAddress
		httpSrv, httpSrvDone, err = zetaserver.StartJSONRPC(ctx, clientCtx, tmRPCAddr, tmEndpoint, &config, idxer)
		if err != nil {
			return err
		}
		defer func() {
			shutdownCtx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelFn()
			if err := httpSrv.Shutdown(shutdownCtx); err != nil {
				logger.Error("HTTP server shutdown produced a warning", "error", err.Error())
			} else {
				logger.Info("HTTP server shut down, waiting 5 sec")
				select {
				case <-time.Tick(5 * time.Second):
				case <-httpSrvDone:
				}
			}
		}()
	}

	// At this point it is safe to block the process if we're in query only mode as
	// we do not need to handle any Tendermint related processes.
	// At this point it is safe to block the process if we're in query only mode as
	// we do not need to start Rosetta or handle any CometBFT related processes.
	return g.Wait()
}

// returns a function which returns the genesis doc from the genesis file.
func GenDocProvider(cfg *cmtcfg.Config) func() (*cmttypes.GenesisDoc, error) {
	return func() (*cmttypes.GenesisDoc, error) {
		appGenesis, err := genutiltypes.AppGenesisFromFile(cfg.GenesisFile())
		if err != nil {
			return nil, err
		}

		return appGenesis.ToGenesisDoc()
	}
}

func startGrpcServer(
	ctx context.Context,
	svrCtx *server.Context,
	clientCtx client.Context,
	g *errgroup.Group,
	config serverconfig.GRPCConfig,
	app sdktypes.Application,
) (*grpc.Server, client.Context, error) {
	if !config.Enable {
		// return grpcServer as nil if gRPC is disabled
		return nil, clientCtx, nil
	}
	_, _, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, clientCtx, errorsmod.Wrapf(err, "invalid grpc address %s", config.Address)
	}

	maxSendMsgSize := config.MaxSendMsgSize
	if maxSendMsgSize == 0 {
		maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
	}

	maxRecvMsgSize := config.MaxRecvMsgSize
	if maxRecvMsgSize == 0 {
		maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
	}

	// if gRPC is enabled, configure gRPC client for gRPC gateway and json-rpc
	grpcClient, err := grpc.NewClient(
		config.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
			grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(maxSendMsgSize),
		),
	)
	if err != nil {
		return nil, clientCtx, err
	}
	// Set `GRPCClient` to `clientCtx` to enjoy concurrent grpc query.
	// only use it if gRPC server is enabled.
	clientCtx = clientCtx.WithGRPCClient(grpcClient)
	svrCtx.Logger.Debug("gRPC client assigned to client context", "address", config.Address)

	grpcSrv, err := servergrpc.NewGRPCServer(clientCtx, app, config)
	if err != nil {
		return nil, clientCtx, err
	}

	// Start the gRPC server in a goroutine. Note, the provided ctx will ensure
	// that the server is gracefully shut down.
	g.Go(func() error {
		return servergrpc.StartGRPCServer(ctx, svrCtx.Logger.With("module", "grpc-server"), config, grpcSrv)
	})
	return grpcSrv, clientCtx, nil
}

func startTelemetry(cfg serverconfig.Config) (*telemetry.Metrics, error) {
	if !cfg.Telemetry.Enabled {
		return nil, nil
	}

	return telemetry.New(cfg.Telemetry)
}

func openDB(_ sdktypes.AppOptions, rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

// OpenIndexerDB opens the custom eth indexer db, using the same db backend as the main app
func OpenIndexerDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("evmindexer", backendType, dataDir)
}

// getGenDocProvider returns a function which returns the genesis doc from the genesis file.
func getGenDocProvider(cfg *cmtcfg.Config) func() (*cmttypes.GenesisDoc, error) {
	return func() (*cmttypes.GenesisDoc, error) {
		appGenesis, err := genutiltypes.AppGenesisFromFile(cfg.GenesisFile())
		if err != nil {
			return nil, err
		}

		return appGenesis.ToGenesisDoc()
	}
}

func getAndValidateConfig(svrCtx *server.Context) (serverconfig.Config, error) {
	config, err := serverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		return config, err
	}

	if err := config.ValidateBasic(); err != nil {
		return config, err
	}
	return config, nil
}

func getCtx(svrCtx *server.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	// listen for quit signals so the calling parent process can gracefully exit
	server.ListenForQuitSignals(g, block, cancelFn, svrCtx.Logger)
	return g, ctx
}

func openTraceWriter(traceWriterFile string) (w io.WriteCloser, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile( //nolint:gosec
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}

func setupTraceWriter(svrCtx *server.Context) (traceWriter io.WriteCloser, cleanup func(), err error) {
	// clean up the traceWriter when the server is shutting down
	cleanup = func() {}

	traceWriterFile := svrCtx.Viper.GetString(flagTraceStore)
	traceWriter, err = openTraceWriter(traceWriterFile)
	if err != nil {
		return traceWriter, cleanup, err
	}

	// if flagTraceStore is not used then traceWriter is nil
	if traceWriter != nil {
		cleanup = func() {
			if err = traceWriter.Close(); err != nil {
				svrCtx.Logger.Error("failed to close trace writer", "err", err)
			}
		}
	}

	return traceWriter, cleanup, nil
}

// emitServerInfoMetrics emits server info related metrics using application telemetry.
func emitServerInfoMetrics() {
	var ls []metrics.Label

	versionInfo := version.NewInfo()
	if len(versionInfo.GoVersion) > 0 {
		ls = append(ls, telemetry.NewLabel("go", versionInfo.GoVersion))
	}
	if len(versionInfo.CosmosSdkVersion) > 0 {
		ls = append(ls, telemetry.NewLabel("version", versionInfo.CosmosSdkVersion))
	}

	if len(ls) == 0 {
		return
	}

	telemetry.SetGaugeWithLabels([]string{"server", "info"}, 1, ls)
}
