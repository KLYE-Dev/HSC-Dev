package core

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/klyed/hivesmartchain/bcm"
	"github.com/klyed/hivesmartchain/bridges"
	"github.com/klyed/hivesmartchain/consensus/abci"
	"github.com/klyed/hivesmartchain/execution"
	"github.com/klyed/hivesmartchain/keys"
	"github.com/klyed/hivesmartchain/logging/structure"
	"github.com/klyed/hivesmartchain/process"
	"github.com/klyed/hivesmartchain/project"
	"github.com/klyed/hivesmartchain/rpc"
	"github.com/klyed/hivesmartchain/rpc/lib/server"
	"github.com/klyed/hivesmartchain/rpc/metrics"
	"github.com/klyed/hivesmartchain/rpc/rpcdump"
	"github.com/klyed/hivesmartchain/rpc/rpcevents"
	"github.com/klyed/hivesmartchain/rpc/rpcinfo"
	"github.com/klyed/hivesmartchain/rpc/rpcquery"
	"github.com/klyed/hivesmartchain/rpc/rpctransact"
	"github.com/klyed/hivesmartchain/rpc/web3"
	"github.com/klyed/hivesmartchain/txs"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/version"
	hex "github.com/tmthrgd/go-hex"
)

const (
	ProfilingProcessName   = "Profiling"
	DatabaseProcessName    = "Database"
	NoConsensusProcessName = "NoConsensusExecution"
	TendermintProcessName  = "Tendermint"
	StartupProcessName     = "StartupAnnouncer"
	Web3ProcessName        = "rpcConfig/web3"
	InfoProcessName        = "rpcConfig/info"
	GRPCProcessName        = "rpcConfig/GRPC"
	MetricsProcessName     = "rpcConfig/metrics"
	BridgeHIVEProcessName  = "Bridges"
)

func DefaultProcessLaunchers(kern *Kernel, rpcConfig *rpc.RPCConfig, keysConfig *keys.KeysConfig) []process.Launcher {
	// Run announcer after Tendermint so it can get some details
	return []process.Launcher{
		ProfileLauncher(kern, rpcConfig.Profiler),
		DatabaseLauncher(kern),
		NoConsensusLauncher(kern),
		TendermintLauncher(kern),
		StartupLauncher(kern),
		Web3Launcher(kern, rpcConfig.Web3),
		InfoLauncher(kern, rpcConfig.Info),
		MetricsLauncher(kern, rpcConfig.Metrics),
		GRPCLauncher(kern, rpcConfig.GRPC, keysConfig),
		Bridges(kern),
	}
}

func ProfileLauncher(kern *Kernel, conf *rpc.ServerConfig) process.Launcher {
	return process.Launcher{
		Name:    ProfilingProcessName,
		Enabled: conf.Enabled,
		Launch: func() (process.Process, error) {
			debugServer := &http.Server{
				Addr: conf.ListenAddress(),
			}
			go func() {
				err := debugServer.ListenAndServe()
				if err != nil {
					kern.Logger.InfoMsg("Error from pprof debug server", structure.ErrorKey, err)
				}
			}()
			return debugServer, nil
		},
	}
}

func Bridges(kern *Kernel) process.Launcher {
	return process.Launcher{
		Name:    BridgeHIVEProcessName,
		Enabled: true,
		Launch: func() (process.Process, error) {
			var HIVE = bridges.Startbridge("start", false, true)
			/*
				hive := &bridges.func{
					"start",
					false
				}
			*/
			defer func() {
				HIVE = bridges.Startbridge("stop", true, false)
			}()
			return process.ShutdownFunc(func(ctx context.Context) error {
				HIVE = bridges.Startbridge("stop", true, false)
				//bridges.Startbridge("stop", true, false) error
				return HIVE
			}), nil
			//return HiveServer, err
			/*
				kern.Logger.InfoMsg(bridges.Startbridge("start"), err)
				return process.ShutdownFunc(func(ctx context.Context) error {
					bridges.Startbridge("stop")
					return nil
					//if err := bridges.Startbridge("stop"); err != nil {
					//	log.Fatalln("Error:", err)
					//}
				}), nil
			*/
			//return bridges.Startbridge
		},
	}
}

func DatabaseLauncher(kern *Kernel) process.Launcher {
	return process.Launcher{
		Name:    DatabaseProcessName,
		Enabled: true,
		Launch: func() (process.Process, error) {
			// Just close database
			return process.ShutdownFunc(func(ctx context.Context) error {
				kern.database.Close()
				return nil
			}), nil
		},
	}
}

// Run a single uncoordinated local state
func NoConsensusLauncher(kern *Kernel) process.Launcher {
	return process.Launcher{
		Name:    NoConsensusProcessName,
		Enabled: kern.Node == nil,
		Launch: func() (process.Process, error) {
			accountState := kern.State
			nameRegState := kern.State
			nodeRegState := kern.State
			validatorSet := kern.State
			kern.Service = rpc.NewService(accountState, nameRegState, nodeRegState, kern.Blockchain, validatorSet, nil, kern.Logger)
			// TimeoutFactor scales in units of seconds
			blockDuration := time.Duration(kern.timeoutFactor * float64(time.Second))
			//proc := abci.NewProcess(kern.checker, kern.committer, kern.Blockchain, kern.txCodec, blockDuration, kern.Panic)
			proc := abci.NewProcess(kern.committer, kern.Blockchain, kern.txCodec, blockDuration, kern.Panic)
			// Provide execution accounts against backend state since we will commit immediately
			accounts := execution.NewAccounts(kern.committer, kern.keyClient, AccountsRingMutexCount)
			// Elide consensus and use a CheckTx function that immediately commits any valid transaction
			kern.Transactor = execution.NewTransactor(kern.Blockchain,
				kern.Emitter, accounts, proc.CheckTx, "", kern.txCodec, kern.Logger)
			return proc, nil
		},
	}
}

func TendermintLauncher(kern *Kernel) process.Launcher {
	return process.Launcher{
		Name:    TendermintProcessName,
		Enabled: kern.Node != nil,
		Launch: func() (process.Process, error) {
			const errHeader = "TendermintLauncher():"
			nodeView, err := kern.GetNodeView()
			if err != nil {
				return nil, fmt.Errorf("%s cannot get NodeView %v", errHeader, err)
			}

			var id p2p.ID
			if ni := nodeView.NodeInfo(); ni != nil {
				id = p2p.ID(ni.ID.Bytes())
			}

			kern.Blockchain.SetBlockStore(bcm.NewBlockStore(nodeView.BlockStore()))
			// Provide execution accounts against checker state so that we can assign sequence numbers
			accounts := execution.NewAccounts(kern.checker, kern.keyClient, AccountsRingMutexCount)
			// Pass transactions to Tendermint's CheckTx function for broadcast and consensus
			checkTx := kern.Node.Mempool().CheckTx
			kern.Transactor = execution.NewTransactor(kern.Blockchain,
				kern.Emitter, accounts, checkTx, id, kern.txCodec, kern.Logger)

			accountState := kern.State
			eventsState := kern.State
			nameRegState := kern.State
			nodeRegState := kern.State
			validatorState := kern.State
			kern.Service = rpc.NewService(accountState, nameRegState, nodeRegState, kern.Blockchain, validatorState, nodeView, kern.Logger)
			kern.EthService = web3.NewEthService(accountState, eventsState, kern.Blockchain, validatorState, nodeView, kern.Transactor, kern.keyStore, kern.Logger)

			if err := kern.Node.Start(); err != nil {
				return nil, fmt.Errorf("%s error starting Tendermint node: %v", errHeader, err)
			}

			return process.ShutdownFunc(func(ctx context.Context) error {
				err := kern.Node.Stop()
				// Close tendermint database connections using our wrapper
				defer kern.Node.Close()
				if err != nil {
					return err
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-kern.Node.Quit():
					kern.Logger.InfoMsg("Tendermint Node has quit, closing DB connections...")
					return nil
				}
			}), nil
		},
	}
}

func StartupLauncher(kern *Kernel) process.Launcher {
	return process.Launcher{
		Name:    StartupProcessName,
		Enabled: true,
		Launch: func() (process.Process, error) {
			start := time.Now()
			shutdown := process.ShutdownFunc(func(ctx context.Context) error {
				stop := time.Now()
				return kern.Logger.InfoMsg("HiveSmartChain is shutting down. Prepare for re-entrancy.",
					"announce", "shutdown",
					"shutdown_time", stop,
					"elapsed_run_time", stop.Sub(start).String())
			})

			if kern.Node == nil {
				return shutdown, nil
			}

			nodeView, err := kern.GetNodeView()
			if err != nil {
				return nil, err
			}

			genesisDoc := kern.Blockchain.GenesisDoc()
			info := kern.Node.NodeInfo()
			netAddress, err := info.NetAddress()
			if err != nil {
				return nil, err
			}
			logger := kern.Logger.With(
				"launch_time", start,
				"hsc_version", project.FullVersion(),
				"tendermint_version", version.TMCoreSemVer,
				"validator_address", nodeView.ValidatorAddress(),
				"node_id", string(info.ID()),
				"net_address", netAddress.String(),
				"genesis_app_hash", genesisDoc.AppHash.String(),
				"genesis_hash", hex.EncodeUpperToString(genesisDoc.Hash()),
			)

			err = logger.InfoMsg("Hive Smart Chain (HSC) v0.0.4 is launching...", "announce", "startup")
			return shutdown, err
		},
	}
}

func InfoLauncher(kern *Kernel, conf *rpc.ServerConfig) process.Launcher {
	return process.Launcher{
		Name:    InfoProcessName,
		Enabled: conf.Enabled,
		Launch: func() (process.Process, error) {
			listener, err := process.ListenerFromAddress(conf.ListenAddress())
			if err != nil {
				return nil, err
			}
			err = kern.registerListener(InfoProcessName, listener)
			if err != nil {
				return nil, err
			}
			server, err := rpcinfo.StartServer(kern.Service, "/websocket", listener, kern.Logger)
			if err != nil {
				return nil, err
			}
			return server, nil
		},
	}
}

func Web3Launcher(kern *Kernel, conf *rpc.ServerConfig) process.Launcher {
	return process.Launcher{
		Name:    Web3ProcessName,
		Enabled: conf.Enabled,
		Launch: func() (process.Process, error) {
			listener, err := process.ListenerFromAddress(fmt.Sprintf("%s:%s", conf.ListenHost, conf.ListenPort))
			if err != nil {
				return nil, err
			}
			err = kern.registerListener(Web3ProcessName, listener)
			if err != nil {
				return nil, err
			}

			srv, err := server.StartHTTPServer(listener, web3.NewServer(kern.EthService), kern.Logger)
			if err != nil {
				return nil, err
			}

			return srv, nil
		},
	}
}

func MetricsLauncher(kern *Kernel, conf *rpc.MetricsConfig) process.Launcher {
	return process.Launcher{
		Name:    MetricsProcessName,
		Enabled: conf.Enabled,
		Launch: func() (process.Process, error) {
			listener, err := process.ListenerFromAddress(conf.ListenAddress())
			if err != nil {
				return nil, err
			}
			err = kern.registerListener(MetricsProcessName, listener)
			if err != nil {
				return nil, err
			}
			server, err := metrics.StartServer(kern.Service, conf.MetricsPath, listener, conf.BlockSampleSize,
				kern.Logger)
			if err != nil {
				return nil, err
			}
			return server, nil
		},
	}
}

func GRPCLauncher(kern *Kernel, conf *rpc.ServerConfig, keyConfig *keys.KeysConfig) process.Launcher {
	return process.Launcher{
		Name:    GRPCProcessName,
		Enabled: conf.Enabled,
		Launch: func() (process.Process, error) {
			nodeView, err := kern.GetNodeView()
			if err != nil {
				return nil, err
			}

			listener, err := process.ListenerFromAddress(conf.ListenAddress())
			if err != nil {
				return nil, err
			}
			err = kern.registerListener(GRPCProcessName, listener)
			if err != nil {
				return nil, err
			}

			grpcServer := rpc.NewGRPCServer(kern.Logger)
			var ks *keys.FilesystemKeyStore
			if kern.keyStore != nil {
				ks = kern.keyStore
			}
			grpcServer.GetServiceInfo()

			if keyConfig.GRPCServiceEnabled {
				if kern.keyStore == nil {
					ks = keys.NewFilesystemKeyStore(keyConfig.KeysDirectory, keyConfig.AllowBadFilePermissions)
				}
				keys.RegisterKeysServer(grpcServer, ks)
			}
			rpcquery.RegisterQueryServer(grpcServer, rpcquery.NewQueryServer(kern.State, kern.Blockchain, nodeView,
				kern.Logger))

			txCodec := txs.NewProtobufCodec()
			rpctransact.RegisterTransactServer(grpcServer,
				rpctransact.NewTransactServer(kern.State, kern.Blockchain, kern.Transactor, txCodec, kern.Logger))

			rpcevents.RegisterExecutionEventsServer(grpcServer, rpcevents.NewExecutionEventsServer(kern.State,
				kern.Emitter, kern.Blockchain, kern.Logger))

			rpcdump.RegisterDumpServer(grpcServer, rpcdump.NewDumpServer(kern.State, kern.Blockchain, kern.Logger))

			// Provides metadata about services registered
			// reflection.Register(grpcServer)

			go grpcServer.Serve(listener)

			return process.ShutdownFunc(func(ctx context.Context) error {
				grpcServer.Stop()
				// listener is closed for us
				return nil
			}), nil
		},
	}
}
