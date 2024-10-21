package harness

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/babylonlabs-io/babylon/btctxformatter"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btckpttypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type SubReporter struct {
	tm     *TestManager
	client *SenderWithBabylonClient
	wg     *sync.WaitGroup
	quit   chan struct{}
}

func NewSubReporter(
	tm *TestManager,
	client *SenderWithBabylonClient,
) *SubReporter {
	return &SubReporter{
		tm:     tm,
		client: client,
		wg:     &sync.WaitGroup{},
		quit:   make(chan struct{}),
	}
}

func (s *SubReporter) Start() {
	s.wg.Add(1)
	go s.runForever()
}

func (s *SubReporter) Stop() {
	close(s.quit)
	s.wg.Wait()
}

func (s *SubReporter) runForever() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			// last ea
			resp, err := s.client.RawCheckpointList(checkpointingtypes.Sealed, nil)

			if err != nil {
				fmt.Printf("failed to get checkpoints %s\n", err)
				continue
			}

			if len(resp.RawCheckpoints) == 0 {
				continue
			}

			firstSelead := resp.RawCheckpoints[0]

			fmt.Printf("retrieved checkpoint for epoch %d\n", firstSelead.Ckpt.EpochNum)

			if err = s.buildSendReportCheckpoint(firstSelead.Ckpt); err != nil {
				fmt.Printf("err buildSendReportCheckpoint for epoch %d err:%v\n", firstSelead.Ckpt.EpochNum, err)
			}
		}
	}
}

func (s *SubReporter) encodeCheckpointData(ckpt *checkpointingtypes.RawCheckpointResponse) ([]byte, []byte, error) {
	// Convert to raw checkpoint
	rawCkpt, err := ckpt.ToRawCheckpoint()
	if err != nil {
		return nil, nil, err
	}

	// Convert raw checkpoint to BTC checkpoint
	btcCkpt, err := checkpointingtypes.FromRawCkptToBTCCkpt(rawCkpt, s.client.BabylonAddress)
	if err != nil {
		return nil, nil, err
	}

	// Encode checkpoint data
	data1, data2, err := btctxformatter.EncodeCheckpointData(
		babylonTag,
		0,
		btcCkpt,
	)
	if err != nil {
		return nil, nil, err
	}

	// Return the encoded data
	return data1, data2, nil
}

func (s *SubReporter) buildSendReportCheckpoint(ckpt *checkpointingtypes.RawCheckpointResponse) error {
	data1, data2, err := s.encodeCheckpointData(ckpt)
	if err != nil {
		return err
	}

	builder1 := txscript.NewScriptBuilder()
	dataScript1, err := builder1.AddOp(txscript.OP_RETURN).AddData(data1).Script()
	if err != nil {
		return err
	}

	builder2 := txscript.NewScriptBuilder()
	dataScript2, err := builder2.AddOp(txscript.OP_RETURN).AddData(data2).Script()
	if err != nil {
		return err
	}

	dataTx1 := wire.NewMsgTx(2)
	dataTx1.AddTxOut(wire.NewTxOut(0, dataScript1))

	dataTx2 := wire.NewMsgTx(2)
	dataTx2.AddTxOut(wire.NewTxOut(0, dataScript2))

	_, hash1, err := s.tm.AtomicFundSignSendStakingTx(wire.NewTxOut(0, dataScript1))
	if err != nil {
		return err
	}
	_, hash2, err := s.tm.AtomicFundSignSendStakingTx(wire.NewTxOut(0, dataScript2))
	if err != nil {
		return err
	}

	proofs := s.waitFor2TransactionsConfirmation(hash1, hash2, 2)

	if len(proofs) == 0 {
		// we are quiting
		return fmt.Errorf("no proofs")
	}

	fmt.Printf("sending checkpoint for epoch %d with proof %d \n", ckpt.EpochNum, len(proofs))

	msg := &btckpttypes.MsgInsertBTCSpvProof{
		Submitter: s.client.BabylonAddress.String(),
		Proofs:    proofs,
	}

	resp, err := s.client.SendMsgs(context.Background(), []sdk.Msg{msg})
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("send messages nil")
	}

	return nil
}

func (s *SubReporter) waitFor2TransactionsConfirmation(
	txHash *chainhash.Hash,
	txHash2 *chainhash.Hash,
	requiredDepth uint32,
) []*btckpttypes.BTCSpvProof {

	t := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-t.C:
			proof, err := s.buildSpvProof(txHash, txHash2, requiredDepth)
			if err != nil {
				panic(err)
			}

			if proof != nil {
				return proof
			}
		case <-s.quit:
			return nil
		}
	}
}

func (s *SubReporter) buildSpvProof(
	txHash *chainhash.Hash,
	txHash2 *chainhash.Hash,
	requiredDepth uint32) ([]*btckpttypes.BTCSpvProof, error) {
	tx1, err := s.tm.TestRpcClient.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}
	tx2, err := s.tm.TestRpcClient.GetTransaction(txHash2)
	if err != nil {
		return nil, err
	}

	if tx1.Confirmations > int64(requiredDepth)+1 && tx2.Confirmations > int64(requiredDepth)+1 {
		blockHash, err := chainhash.NewHashFromStr(tx1.BlockHash)
		if err != nil {
			return nil, err
		}
		block1, err := s.tm.TestRpcClient.GetBlock(blockHash)
		if err != nil {
			return nil, err
		}
		proof, err := GenerateProof(block1, uint32(tx1.BlockIndex))
		if err != nil {
			return nil, err
		}
		tx1Bytes, err := hex.DecodeString(tx1.Hex)
		if err != nil {
			return nil, err
		}
		block1Header := bbntypes.NewBTCHeaderBytesFromBlockHeader(&block1.Header)

		blockHash2, err := chainhash.NewHashFromStr(tx2.BlockHash)
		if err != nil {
			return nil, err
		}
		block2, err := s.tm.TestRpcClient.GetBlock(blockHash2)
		if err != nil {
			return nil, err
		}
		blco2Header := bbntypes.NewBTCHeaderBytesFromBlockHeader(&block2.Header)
		proof2, err := GenerateProof(block2, uint32(tx2.BlockIndex))
		if err != nil {
			return nil, err
		}
		tx2Bytes, err := hex.DecodeString(tx2.Hex)
		if err != nil {
			return nil, err
		}

		return []*btckpttypes.BTCSpvProof{
			{
				BtcTransaction:      tx1Bytes,
				BtcTransactionIndex: uint32(tx1.BlockIndex),
				MerkleNodes:         proof,
				ConfirmingBtcHeader: &block1Header,
			},
			{
				BtcTransaction:      tx2Bytes,
				BtcTransactionIndex: uint32(tx2.BlockIndex),
				MerkleNodes:         proof2,
				ConfirmingBtcHeader: &blco2Header,
			},
		}, nil
	}

	return nil, nil
}