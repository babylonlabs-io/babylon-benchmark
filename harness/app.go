package harness

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	bncfg "github.com/babylonlabs-io/babylon/client/config"
	"github.com/btcsuite/btcd/btcec/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	delegationsSentCounter  int32
	totalDelegationExecTime int64
)

func Run(ctx context.Context, cfg config.Config) error {
	return startHarness(ctx, cfg)
}

func RunRemote(ctx context.Context, cfg config.Config) error {
	return startRemoteHarness(ctx, cfg)
}

func startRemoteHarness(cmdCtx context.Context, cfg config.Config) error {
	fmt.Println("🚀 Starting remote harness...")
	fmt.Printf("📁 Key path from config: %q\n", cfg.BabylonPath)

	btcClient, err := NewBTCClient(defaultConfig().BTC)
	if err != nil {
		return fmt.Errorf("error creating btc client: %w", err)
	}
	fmt.Println("✅ BTC client created")

	bbncfg := bncfg.DefaultBabylonConfig()
	bbncfg.RPCAddr = cfg.BabylonRPC
	bbncfg.GRPCAddr = cfg.BabylonGRPC
	fmt.Printf("📡 Connecting to Babylon at RPC: %s, GRPC: %s\n", cfg.BabylonRPC, cfg.BabylonGRPC)

	bbnClient, err := New(&bbncfg)
	if err != nil {
		return fmt.Errorf("error creating babylon client: %w", err)
	}
	fmt.Println("✅ Babylon client created")

	fmt.Println("🔑 Importing keys...")
	fmt.Printf("  Reading keys from path: %s\n", cfg.BabylonPath)
	err = bbnClient.importKeys(cfg.KeysPath)
	if err != nil {
		fmt.Printf("❌ Failed to import keys: %v\n", err)
		return fmt.Errorf("error importing keys: %w", err)
	}
	fmt.Println("✅ Keys imported successfully")

	fmt.Println("⚙️ Setting up BTC client...")
	if err := btcClient.Setup(cfg); err != nil {
		return fmt.Errorf("error starting btc client: %w", err)
	}
	fmt.Println("✅ BTC client setup complete")

	fmt.Println("🚀 Starting Babylon client...")
	err = bbnClient.Start()
	if err != nil {
		fmt.Printf("❌ Failed to start Babylon client: %v\n", err)
		return fmt.Errorf("error starting the babylon client: %w", err)
	}
	fmt.Println("✅ Babylon client started successfully")

	fmt.Printf("👥 Creating %d stakers...\n", cfg.TotalStakers)
	var stakers []*BTCStaker
	for i := 0; i < cfg.TotalStakers; i++ {
		fmt.Printf("  Creating staker %d/%d...\n", i+1, cfg.TotalStakers)
		stakerSender, err := NewSenderWithBabylonClient(cmdCtx, fmt.Sprintf("staker-%d", i), cfg.BabylonRPC, cfg.BabylonGRPC)
		if err != nil {
			return fmt.Errorf("failed to create staker sender: %w", err)
		}
		fmt.Printf("  ✅ Staker sender created for staker %d\n", i+1)

		fmt.Printf("  Getting activated height for staker %d...\n", i+1)
		height, err := bbnClient.QueryClient.ActivatedHeight()
		if err != nil {
			return fmt.Errorf("could not get activated height %v", err)
		}
		fmt.Printf("  ✅ Got activated height: %d\n", height.Height)

		fmt.Printf("  Getting finality providers for staker %d...\n", i+1)
		fps, err := bbnClient.QueryClient.ActiveFinalityProvidersAtHeight(height.Height, nil)
		if err != nil {
			return fmt.Errorf("failed to get finality providers: %w", err)
		}

		if len(fps.FinalityProviders) == 0 {
			return fmt.Errorf("no active finality providers found at height %d", height.Height)
		}
		fmt.Printf("  ✅ Found %d finality providers\n", len(fps.FinalityProviders))

		fpPks := make([]*btcec.PublicKey, 0, len(fps.FinalityProviders))
		for _, fp := range fps.FinalityProviders {
			pk, err := fp.BtcPkHex.ToBTCPK()
			if err != nil {
				return fmt.Errorf("failed to convert finality provider key: %w", err)
			}
			fpPks = append(fpPks, pk)
		}

		staker := NewBTCStaker(btcClient, stakerSender, fpPks, nil, nil)
		fmt.Printf("  ✅ Created BTC staker %d\n", i+1)
		stakers = append(stakers, staker)
	}

	fmt.Println("🎯 Starting stakers in batches...")
	go startStakersInBatches(cmdCtx, stakers)

	fmt.Println("✅ Remote harness setup complete")

	// Keep the program running and monitor blocks
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()

		var lastHeight uint64
		for {
			select {
			case <-t.C:
				height, err := bbnClient.QueryClient.ActivatedHeight()
				if err != nil {
					fmt.Printf("🚫 Failed to get height: %v\n", err)
					continue
				}

				if height.Height > lastHeight {
					fmt.Printf("📈 New block! Height: %d (increased by %d)\n",
						height.Height, height.Height-lastHeight)
				} else {
					fmt.Printf("⏳ Current height: %d (unchanged)\n", height.Height)
				}
				lastHeight = height.Height
			case <-cmdCtx.Done():
				return
			}
		}
	}()

	// Wait for context cancellation
	<-cmdCtx.Done()
	return nil
}

func startHarness(cmdCtx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithCancel(cmdCtx)
	defer cancel()

	numStakers := cfg.TotalStakers
	numFinalityProviders := cfg.TotalFinalityProviders
	stopChan := make(chan struct{}) // for stopping when we reach totalDelegations

	tm, err := StartManager(ctx, cfg.NumMatureOutputs, 5, cfg)
	if err != nil {
		return err
	}
	defer tm.Stop()

	// bold text
	fmt.Printf("🟢 Starting with \033[1m%d\033[0m stakers, \u001B[1m%d\u001B[0m finality providers.\n", numStakers, numFinalityProviders)

	cpSender, err := NewSenderWithBabylonClient(ctx, "node0", tm.Config.Babylon0.RPCAddr, tm.Config.Babylon0.GRPCAddr)
	if err != nil {
		return err
	}
	headerSender, err := NewSenderWithBabylonClient(ctx, "headerreporter", tm.Config.Babylon0.RPCAddr, tm.Config.Babylon0.GRPCAddr)
	if err != nil {
		return err
	}
	vigilanteSender, err := NewSenderWithBabylonClient(ctx, "vigilante", tm.Config.Babylon0.RPCAddr, tm.Config.Babylon0.GRPCAddr)
	if err != nil {
		return err
	}

	fpmSender, err := NewSenderWithBabylonClient(ctx, "fpmsender", tm.Config.Babylon0.RPCAddr, tm.Config.Babylon0.GRPCAddr)
	if err != nil {
		return err
	}

	if err := tm.fundAllParties(ctx, []*SenderWithBabylonClient{cpSender, headerSender, vigilanteSender, fpmSender}); err != nil {
		return err
	}

	fpMgrHome, err := tempDir()
	if err != nil {
		return err
	}
	defer cleanupDir(fpMgrHome)

	eotsDir, err := tempDir()
	if err != nil {
		return err
	}
	defer cleanupDir(eotsDir)

	gen := NewBTCHeaderGenerator(tm, headerSender)
	gen.Start(ctx)

	vig := NewSubReporter(tm, vigilanteSender)
	vig.Start(ctx)

	fpMgr := NewFinalityProviderManager(tm, fpmSender, zap.NewNop(), numFinalityProviders, fpMgrHome, eotsDir)
	if err = fpMgr.Initialize(ctx, cfg.NumPubRand); err != nil {
		return err
	}

	var stakers []*BTCStaker
	for i := 0; i < numStakers; i++ {
		stakerSender, err := NewSenderWithBabylonClient(ctx, fmt.Sprintf("staker-%d", i), tm.Config.Babylon1.RPCAddr, tm.Config.Babylon1.GRPCAddr)
		if err != nil {
			return err
		}

		rndFpChunk := fpMgr.getRandomChunk(3)

		stakers = append(stakers, NewBTCStaker(tm.TestRpcClient, stakerSender, rndFpChunk, tm.fundingRequests, tm.fundingResponse))
	}

	// periodically check if we need to fund the staker
	go tm.fundForever(ctx)

	// fund all stakers
	if err := tm.fundAllParties(ctx, senders(stakers)); err != nil {
		return err
	}

	// start stakers
	if err := startStakersInBatches(ctx, stakers); err != nil {
		return err
	}

	go printStatsForever(ctx, tm, stopChan, cfg)

	covenantSender, err := NewSenderWithBabylonClient(ctx, "covenant", tm.Config.Babylon0.RPCAddr, tm.Config.Babylon0.GRPCAddr)
	if err != nil {
		return err
	}
	covenant := NewCovenantEmulator(tm, container.CovenantPrivKey, covenantSender)
	if err := tm.fundAllParties(ctx, []*SenderWithBabylonClient{covenantSender}); err != nil {
		return err
	}

	covenant.Start(ctx)

	// finality providers start voting
	fpMgr.Start(ctx)

	go tm.listBlocksForever(ctx)

	select {
	case <-ctx.Done():
	case <-stopChan:
		return nil
	}

	return nil
}
func printStatsForever(ctx context.Context, tm *TestManager, stopChan chan struct{}, cfg config.Config) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()

	prevSent := int32(0)
	prevTime := time.Now()

	for {
		select {
		case <-t.C:
			currentSent := atomic.LoadInt32(&delegationsSentCounter)
			if currentSent == 0 || currentSent == prevSent {
				continue
			}

			if cfg.TotalDelegations != 0 && currentSent >= int32(cfg.TotalDelegations) {
				fmt.Printf("🟩 Reached desired total delegation %d, stopping the CLI...\n", currentSent)
				close(stopChan)
			}

			mem, err := tm.manger.MemoryUsage(ctx, "babylond-node0")
			if err != nil {
				fmt.Printf("err getting memory usage for bbn node %v\n", err)
			}

			now := time.Now()
			delegationsPerSecond := float64(currentSent-prevSent) / now.Sub(prevTime).Seconds()
			fmt.Printf("📄 Delegations sent: %d, rate: %.2f delegations/sec, ts: %s, mem: %d MB\n",
				currentSent, delegationsPerSecond, now.Format(time.UnixDate), mem/1e6)
			fmt.Printf("⏱️ Average delegation submission time: %.4f seconds\n", avgExecutionTime())

			prevSent = currentSent
			prevTime = now
		case <-ctx.Done():
			return
		}
	}
}

// avgExecutionTime calculates the average execution time in seconds
func avgExecutionTime() float64 {
	totalTime := atomic.LoadInt64(&totalDelegationExecTime)
	count := atomic.LoadInt32(&delegationsSentCounter)

	if count == 0 {
		return 0
	}

	return float64(totalTime) / float64(count) / 1e9
}

func startStakersInBatches(ctx context.Context, stakers []*BTCStaker) error {
	const (
		batchSize     = 25
		batchInterval = 2 * time.Second
	)

	fmt.Printf("⌛ Starting %d stakers in batches of %d, with %s interval\n",
		len(stakers), batchSize, batchInterval)

	start := time.Now()
	var g errgroup.Group
	for i := 0; i < len(stakers); i += batchSize {
		end := i + batchSize
		if end > len(stakers) {
			end = len(stakers)
		}
		batch := stakers[i:end]

		g.Go(func() error {
			return startBatch(ctx, batch)
		})

		// Wait before starting the next batch, unless it's the last batch
		if end < len(stakers) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(batchInterval):
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("✅ All %d stakers started in %s\n", len(stakers), elapsed)

	return g.Wait()
}

func startBatch(ctx context.Context, batch []*BTCStaker) error {
	var g errgroup.Group
	for _, staker := range batch {
		g.Go(func() error {
			return staker.Start(ctx)
		})
	}
	return g.Wait()
}
