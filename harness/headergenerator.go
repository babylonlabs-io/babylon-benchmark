package harness

import (
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"sync"
	"time"
)

type BTCHeaderGenerator struct {
	tm     *TestManager
	client *SenderWithBabylonClient
	wg     *sync.WaitGroup
	quit   chan struct{}
}

func NewBTCHeaderGenerator(
	tm *TestManager,
	client *SenderWithBabylonClient) *BTCHeaderGenerator {
	return &BTCHeaderGenerator{
		tm:     tm,
		client: client,
		wg:     &sync.WaitGroup{},
		quit:   make(chan struct{}),
	}
}

func (s *BTCHeaderGenerator) CatchUpBTCLightClient() error {
	btcHeight, err := s.tm.TestRpcClient.GetBlockCount()
	if err != nil {
		return err
	}

	tipResp, err := s.client.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	btclcHeight := tipResp.Header.Height

	var headers []*wire.BlockHeader
	for i := int(btclcHeight + 1); i <= int(btcHeight); i++ {
		hash, err := s.tm.TestRpcClient.GetBlockHash(int64(i))
		if err != nil {
			return err
		}
		header, err := s.tm.TestRpcClient.GetBlockHeader(hash)
		if err != nil {
			return err
		}
		headers = append(headers, header)
	}

	_, err = s.client.InsertBTCHeadersToBabylon(headers)
	if err != nil {
		return err
	}

	return nil
}

func (g *BTCHeaderGenerator) Start() {
	if err := g.CatchUpBTCLightClient(); err != nil {
		panic(err)
	}
	g.wg.Add(1)
	go g.runForever()
}

func (g *BTCHeaderGenerator) Stop() {
	close(g.quit)
	g.wg.Wait()
}

func (g *BTCHeaderGenerator) runForever() {
	defer g.wg.Done()

	t := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-g.quit:
			return
		case <-t.C:
			if err := g.genBlocks(); err != nil {
				panic(err)
			}
		}
	}
}

func (g *BTCHeaderGenerator) genBlocks() error {
	resp := g.tm.BitcoindHandler.GenerateBlocks(1)
	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	if err != nil {
		return err
	}
	block, err := g.tm.TestRpcClient.GetBlock(hash)
	if err != nil {
		return err
	}
	_, err = g.client.InsertBTCHeadersToBabylon([]*wire.BlockHeader{&block.Header})
	if err != nil {
		return err
	}

	btcHeight, err := g.tm.TestRpcClient.GetBlockCount()
	if err != nil {
		return err
	}

	tipResp, err := g.client.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	btclcHeight := tipResp.Header.Height

	fmt.Printf("Current best block height: %d, BTC light client height: %d\n", btcHeight, btclcHeight)

	return nil
}
