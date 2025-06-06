package harness

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

var (
	delegationsSentCounter  int32
	totalDelegationExecTime int64
)

func Run(ctx context.Context, cfg config.Config) error {
	return startHarness(ctx, cfg)
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

		stakers = append(stakers, NewBTCStaker(tm, stakerSender, rndFpChunk, tm.fundingRequests, tm.fundingResponse))
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
