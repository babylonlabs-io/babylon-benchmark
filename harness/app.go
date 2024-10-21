package harness

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"time"
)

func Run(ctx context.Context) error {
	return startHarness(ctx)
}

func startHarness(ctx context.Context) error {
	numMatureOutputs := uint32(500)
	tm, err := StartManager(numMatureOutputs, 10)
	if err != nil {
		return err
	}
	defer tm.Stop()

	cpSender, err := NewSenderWithBabylonClient("node0", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	headerSender, err := NewSenderWithBabylonClient("headerreporter", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	vigilanteSender, err := NewSenderWithBabylonClient("vigilante", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}

	if err := tm.fundAllParties([]*SenderWithBabylonClient{cpSender, headerSender, vigilanteSender}); err != nil {
		return err
	}

	fpResp, fpInfo, err := cpSender.CreateFinalityProvider()
	if err != nil {
		return err
	}
	fmt.Println(fpResp)
	fmt.Println(err)

	numStakers := 50

	var stakers []*BTCStaker
	for i := 0; i < numStakers; i++ {
		stakerSender, err := NewSenderWithBabylonClient(fmt.Sprintf("staker%d", i), tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
		if err != nil {
			return err
		}
		staker := NewBTCStaker(tm, stakerSender, fpInfo.BtcPk.MustToBTCPK())
		stakers = append(stakers, staker)
	}

	// fund all stakers
	if err := tm.fundAllParties(senders(stakers)); err != nil {
		return err
	}

	gen := NewBTCHeaderGenerator(tm, headerSender)
	gen.Start()
	defer gen.Stop()

	vig := NewSubReporter(tm, vigilanteSender)
	vig.Start()
	defer vig.Stop()

	// start stakers and defer stops
	// TODO: Ideally stakers would start on different times to reduce contention
	// on funding BTC wallet
	for _, staker := range stakers {
		if err := staker.Start(); err != nil {
			return err
		}
		defer staker.Stop()
	}

	covenantSender, err := NewSenderWithBabylonClient("covenant", tm.Config.Babylon.RPCAddr, tm.Config.Babylon.GRPCAddr)
	if err != nil {
		return err
	}
	covenant := NewCovenantEmulator(tm, container.CovenantPrivKey, covenantSender)
	if err := tm.fundAllParties([]*SenderWithBabylonClient{covenantSender}); err != nil {
		return err
	}

	covenant.Start()
	defer covenant.Stop()

	fmt.Printf("sleeping 120s")
	time.Sleep(120 * time.Second)

	<-ctx.Done()

	return nil
}
