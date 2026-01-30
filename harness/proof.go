package harness

import (
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	"github.com/btcsuite/btcd/wire"
)

func GenerateProof(block *wire.MsgBlock, txIdx uint32) ([]byte, error) {
	headerBytes := bbntypes.NewBTCHeaderBytesFromBlockHeader(&block.Header)

	var txsBytes [][]byte
	for _, tx := range block.Transactions {
		bytes, err := bbntypes.SerializeBTCTx(tx)

		if err != nil {
			return nil, err
		}

		txsBytes = append(txsBytes, bytes)
	}

	proof, err := btcctypes.SpvProofFromHeaderAndTransactions(&headerBytes, txsBytes, uint(txIdx))

	if err != nil {
		return nil, err
	}

	return proof.MerkleNodes, nil
}
