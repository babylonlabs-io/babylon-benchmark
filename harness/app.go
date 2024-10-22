package harness

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"go.uber.org/zap"
)

func Run(ctx context.Context) error {
	return startHarness(ctx)
}

func startHarness(ctx context.Context) error {
	numMatureOutputs := uint32(500)
	tm, err := StartManager(ctx, numMatureOutputs, 5)
	if err != nil {
		return err
	}
	defer tm.Stop()

	cpSender, err := NewSenderWithBabylonClient(ctx, "node0", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	headerSender, err := NewSenderWithBabylonClient(ctx, "headerreporter", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	vigilanteSender, err := NewSenderWithBabylonClient(ctx, "vigilante", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}

	fpmSender, err := NewSenderWithBabylonClient(ctx, "fpmsender", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
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

	fpMgr := NewFinalityProviderManager(tm, fpmSender, zap.NewNop(), 3, fpMgrHome, eotsDir) // todo(lazar); fp count cfg
	if err = fpMgr.Initialize(ctx); err != nil {
		return err
	}

	numStakers := 50

	var stakers []*BTCStaker
	for i := 0; i < numStakers; i++ {
		stakerSender, err := NewSenderWithBabylonClient(ctx, fmt.Sprintf("staker%d", i), tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
		if err != nil {
			return err
		}
		stakers = append(stakers, NewBTCStaker(tm, stakerSender, fpMgr.randomFp().btcPk.MustToBTCPK()))
	}

	// fund all stakers
	if err := tm.fundAllParties(ctx, senders(stakers)); err != nil {
		return err
	}

	// start stakers and defer stops
	// TODO(lazar): Ideally stakers would start on different times to reduce contention
	// on funding BTC wallet
	for _, staker := range stakers {
		if err := staker.Start(ctx); err != nil {
			return err
		}
	}

	covenantSender, err := NewSenderWithBabylonClient(ctx, "covenant", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	covenant := NewCovenantEmulator(tm, container.CovenantPrivKey, covenantSender)
	if err := tm.fundAllParties(ctx, []*SenderWithBabylonClient{covenantSender}); err != nil {
		return err
	}

	covenant.Start(ctx)

	// start voting
	fpMgr.Start(ctx)

	<-ctx.Done()

	return nil
}
