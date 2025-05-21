package harness

import (
	"context"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"slices"
)

type BTCHeaderGenerator struct {
	tm     *TestManager
	client *SenderWithBabylonClient
}

func NewBTCHeaderGenerator(
	tm *TestManager,
	client *SenderWithBabylonClient) *BTCHeaderGenerator {
	return &BTCHeaderGenerator{
		tm:     tm,
		client: client,
	}
}

func (s *BTCHeaderGenerator) CatchUpBTCLightClient(ctx context.Context) error {
	btcHeight, err := s.tm.TestRpcClient.GetBlockCount()
	if err != nil {
		return err
	}

	tipResp, err := s.client.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	btclcHeight := tipResp.Header.Height

	headers := make([]*wire.BlockHeader, 0, btcHeight)
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

	for headersChunk := range slices.Chunk(headers, 150) {
		if err = s.client.InsertBTCHeadersToBabylon(ctx, headersChunk); err != nil {
			return err
		}
	}

	return nil
}

func (g *BTCHeaderGenerator) Start(ctx context.Context) {
	if err := g.CatchUpBTCLightClient(ctx); err != nil {
		fmt.Printf("🚫 err catchup light client %v\n", err)
	}
	go g.runForever(ctx)
}

func (g *BTCHeaderGenerator) runForever(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := g.genBlocks(ctx); err != nil {
				fmt.Printf("🚫 err generating blocks: %v\n", err)
			}
		}
	}
}

func (g *BTCHeaderGenerator) genBlocks(ctx context.Context) error {
	resp := g.tm.BitcoindHandler.GenerateBlocks(ctx, 1)
	if len(resp.Blocks) == 0 {
		return fmt.Errorf("generated block is empty")
	}
	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	if err != nil {
		return err
	}
	block, err := g.tm.TestRpcClient.GetBlock(hash)
	if err != nil {
		return err
	}
	err = g.client.InsertBTCHeadersToBabylon(ctx, []*wire.BlockHeader{&block.Header})
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

	fmt.Printf("🧱 Current best block height: %d, BTC light client height: %d\n", btcHeight, btclcHeight)

	return nil
}
