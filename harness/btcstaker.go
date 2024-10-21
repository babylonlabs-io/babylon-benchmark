package harness

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	staking "github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/crypto/bip322"
	bbn "github.com/babylonlabs-io/babylon/types"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btckpttypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	btcstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"sync"
	"time"
)

type BTCStaker struct {
	tm     *TestManager
	client *SenderWithBabylonClient
	fpPK   *btcec.PublicKey
	wg     *sync.WaitGroup
	quit   chan struct{}
}

func NewBTCStaker(
	tm *TestManager,
	client *SenderWithBabylonClient,
	finalityProviderPublicKey *btcec.PublicKey,
) *BTCStaker {
	return &BTCStaker{
		tm:     tm,
		client: client,
		fpPK:   finalityProviderPublicKey,
		wg:     &sync.WaitGroup{},
		quit:   make(chan struct{}),
	}
}

func (s *BTCStaker) Start() error {
	stakerAddress, err := s.tm.TestRpcClient.GetNewAddress("")
	if err != nil {
		return err
	}
	stakerInfo, err := s.tm.TestRpcClient.GetAddressInfo(stakerAddress.String())
	if err != nil {
		return err
	}

	stakerPubKey, err := hex.DecodeString(*stakerInfo.PubKey)
	if err != nil {
		return err
	}
	pk, err := btcec.ParsePubKey(stakerPubKey)
	if err != nil {
		return err
	}

	s.wg.Add(1)
	go s.runForever(stakerAddress, pk)

	return nil
}

func (s *BTCStaker) Stop() {
	close(s.quit)
	s.wg.Wait()
}

// infinite loop to constantly send delegations
func (s *BTCStaker) runForever(stakerAddress btcutil.Address, stakerPk *btcec.PublicKey) {
	defer s.wg.Done()

	for {
		select {
		case <-s.quit:
			return
		default:
			paramsResp, err := s.client.BTCStakingParams()
			if err != nil {
				panic(err)
			}
			s.buildAndSendStakingTransaction(stakerAddress, stakerPk, &paramsResp.Params)
		}
	}
}

func (s *BTCStaker) NewBabylonBip322Pop(
	msg []byte,
	w wire.TxWitness,
	a btcutil.Address) (*btcstypes.ProofOfPossessionBTC, error) {
	err := bip322.Verify(msg, w, a, nil)
	if err != nil {
		return nil, err
	}
	serializedWitness, err := bip322.SerializeWitness(w)
	if err != nil {
		return nil, err
	}
	bip322Sig := btcstypes.BIP322Sig{
		Sig:     serializedWitness,
		Address: a.EncodeAddress(),
	}
	m, err := bip322Sig.Marshal()
	if err != nil {
		return nil, err
	}
	pop := &btcstypes.ProofOfPossessionBTC{
		BtcSigType: btcstypes.BTCSigType_BIP322,
		BtcSig:     m,
	}

	return pop, nil
}

func (s *BTCStaker) signBip322NativeSegwit(stakerAddress btcutil.Address) (*btcstypes.ProofOfPossessionBTC, error) {
	babylonAddrHash := tmhash.Sum(s.client.BabylonAddress.Bytes())

	toSpend, err := bip322.GetToSpendTx(babylonAddrHash, stakerAddress)

	if err != nil {
		return nil, fmt.Errorf("failed to bip322 to spend tx: %w", err)
	}

	if !txscript.IsPayToWitnessPubKeyHash(toSpend.TxOut[0].PkScript) {
		return nil, fmt.Errorf("Bip322NativeSegwit support only native segwit addresses")
	}

	toSpendhash := toSpend.TxHash()

	toSign := bip322.GetToSignTx(toSpend)

	amt := float64(0)
	signed, all, err := s.tm.TestRpcClient.SignRawTransactionWithWallet2(toSign, []btcjson.RawTxWitnessInput{
		{
			Txid:         toSpendhash.String(),
			Vout:         0,
			ScriptPubKey: hex.EncodeToString(toSpend.TxOut[0].PkScript),
			Amount:       &amt,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to sign raw transaction while creating bip322 signature: %w", err)
	}

	if !all {
		return nil, fmt.Errorf("failed to create bip322 signature")
	}

	return s.NewBabylonBip322Pop(
		babylonAddrHash,
		signed.TxIn[0].Witness,
		stakerAddress,
	)
}

type SpendPathDescription struct {
	ControlBlock *txscript.ControlBlock
	ScriptLeaf   *txscript.TapLeaf
}

type TaprootSigningRequest struct {
	FundingOutput    *wire.TxOut
	TxToSign         *wire.MsgTx
	SpendDescription *SpendPathDescription
}

// TaprootSigningResult contains result of signing taproot spend through bitcoind
// wallet. It will contain either Signature or FullInputWitness, never both.
type TaprootSigningResult struct {
	Signature        *schnorr.Signature
	FullInputWitness wire.TxWitness
}

func (s *BTCStaker) SignOneInputTaprootSpendingTransaction(
	stakerPubKey *btcec.PublicKey,
	request *TaprootSigningRequest,
) (*TaprootSigningResult, error) {
	if len(request.TxToSign.TxIn) != 1 {
		return nil, fmt.Errorf("cannot sign transaction with more than one input")
	}

	if !txscript.IsPayToTaproot(request.FundingOutput.PkScript) {
		return nil, fmt.Errorf("cannot sign transaction spending non-taproot output")
	}

	psbtPacket, err := psbt.New(
		[]*wire.OutPoint{&request.TxToSign.TxIn[0].PreviousOutPoint},
		request.TxToSign.TxOut,
		request.TxToSign.Version,
		request.TxToSign.LockTime,
		[]uint32{request.TxToSign.TxIn[0].Sequence},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create PSBT packet with transaction to sign: %w", err)
	}

	psbtPacket.Inputs[0].SighashType = txscript.SigHashDefault
	psbtPacket.Inputs[0].WitnessUtxo = request.FundingOutput
	psbtPacket.Inputs[0].Bip32Derivation = []*psbt.Bip32Derivation{
		{
			PubKey: stakerPubKey.SerializeCompressed(),
		},
	}

	ctrlBlockBytes, err := request.SpendDescription.ControlBlock.ToBytes()

	if err != nil {
		return nil, fmt.Errorf("failed to serialize control block: %w", err)
	}

	psbtPacket.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
		{
			ControlBlock: ctrlBlockBytes,
			Script:       request.SpendDescription.ScriptLeaf.Script,
			LeafVersion:  request.SpendDescription.ScriptLeaf.LeafVersion,
		},
	}

	psbtEncoded, err := psbtPacket.B64Encode()

	if err != nil {
		return nil, fmt.Errorf("failed to encode PSBT packet: %w", err)
	}

	sign := true
	signResult, err := s.tm.TestRpcClient.WalletProcessPsbt(
		psbtEncoded,
		&sign,
		"DEFAULT",
		nil,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to sign PSBT packet: %w", err)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(signResult.Psbt)

	if err != nil {
		return nil, fmt.Errorf("failed to decode signed PSBT packet from b64: %w", err)
	}

	decodedPsbt, err := psbt.NewFromRawBytes(bytes.NewReader(decodedBytes), false)

	if err != nil {
		return nil, fmt.Errorf("failed to decode signed PSBT packet from bytes: %w", err)
	}

	// In our signing request we only handle transaction with one input, and request
	// signature for one public key, thus we can receive at most one signature from btc
	if len(decodedPsbt.Inputs[0].TaprootScriptSpendSig) == 1 {
		schnorSignature := decodedPsbt.Inputs[0].TaprootScriptSpendSig[0].Signature

		parsedSignature, err := schnorr.ParseSignature(schnorSignature)

		if err != nil {
			return nil, fmt.Errorf("failed to parse schnorr signature in psbt packet: %w", err)
		}

		return &TaprootSigningResult{
			Signature: parsedSignature,
		}, nil
	}

	// decodedPsbt.Inputs[0].TaprootScriptSpendSig was 0, it is possible that script
	// required only one signature to build whole witness
	if len(decodedPsbt.Inputs[0].FinalScriptWitness) > 0 {
		// we go whole witness, return it to the caller
		witness, err := bip322.SimpleSigToWitness(decodedPsbt.Inputs[0].FinalScriptWitness)

		if err != nil {
			return nil, fmt.Errorf("failed to parse witness in psbt packet: %w", err)
		}

		return &TaprootSigningResult{
			FullInputWitness: witness,
		}, nil
	}

	// neither witness, nor signature is filled.
	return nil, fmt.Errorf("no signature found in PSBT packet. Wallet can't sign given tx")
}

func (s *BTCStaker) waitForTransactionConfirmation(
	txHash *chainhash.Hash,
	requiredDepth uint32,
) *bstypes.InclusionProof {

	t := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-t.C:
			proof, err := s.buildInclusion(txHash, requiredDepth)
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

func (s *BTCStaker) buildInclusion(
	txHash *chainhash.Hash,
	requiredDepth uint32,
) (*bstypes.InclusionProof, error) {
	tx, err := s.tm.TestRpcClient.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}
	// add + 1 to be sure babylon light client is updated to correct height
	if tx.Confirmations > int64(requiredDepth)+1 {
		blockHash, err := chainhash.NewHashFromStr(tx.BlockHash)
		if err != nil {
			return nil, err
		}
		block, err := s.tm.TestRpcClient.GetBlock(blockHash)
		if err != nil {
			return nil, err
		}
		proof, err := GenerateProof(block, uint32(tx.BlockIndex))
		if err != nil {
			return nil, err
		}

		headerHAsh := bbntypes.NewBTCHeaderHashBytesFromChainhash(blockHash)

		return &bstypes.InclusionProof{
			Key: &btckpttypes.TransactionKey{
				Hash:  &headerHAsh,
				Index: uint32(tx.BlockIndex),
			},
			Proof: proof,
		}, nil
	}
	return nil, nil
}

func (s *BTCStaker) buildAndSendStakingTransaction(
	stakerAddress btcutil.Address,
	stakerPk *btcec.PublicKey,
	params *btcstypes.Params,
) error {
	unbondingTime := uint16(100)
	covKeys, err := bbnPksToBtcPks(params.CovenantPks)
	if err != nil {
		return err
	}

	stakingInfo, err := staking.BuildStakingInfo(
		stakerPk,
		[]*btcec.PublicKey{s.fpPK},
		covKeys,
		params.CovenantQuorum,
		uint16(params.MaxStakingTimeBlocks),
		btcutil.Amount(params.MinStakingValueSat),
		regtestParams,
	)
	if err != nil {
		return err
	}

	stakingTx, hash, err := s.tm.AtomicFundSignSendStakingTx(stakingInfo.StakingOutput)
	fmt.Printf("send staking tx with hash %s \n", hash)

	// TODO: hardcoded two in tests
	inclusionProof := s.waitForTransactionConfirmation(hash, 2)

	if inclusionProof == nil {
		// we are quiting
		return fmt.Errorf("empty inclusion proof")
	}

	fmt.Printf("staking tx confirmed with hash %s \n", hash)

	unbondingTxValue := params.MinStakingValueSat - params.UnbondingFeeSat

	serializedStakingTx, err := bbntypes.SerializeBTCTx(stakingTx)
	if err != nil {
		return err
	}

	unbondingTx := wire.NewMsgTx(2)

	unbondingTx.AddTxIn(wire.NewTxIn(
		wire.NewOutPoint(hash, 0),
		nil,
		nil,
	))
	unbondingInfo, err := staking.BuildUnbondingInfo(
		stakerPk,
		[]*btcec.PublicKey{s.fpPK},
		covKeys,
		params.CovenantQuorum,
		unbondingTime,
		btcutil.Amount(unbondingTxValue),
		regtestParams,
	)

	if err != nil {
		return err
	}
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	serializedUnbondingTx, err := bbntypes.SerializeBTCTx(unbondingTx)
	if err != nil {
		return err
	}

	// build slashing for staking and unbondidn
	stakingSlashing, err := staking.BuildSlashingTxFromStakingTxStrict(
		stakingTx,
		0,
		params.SlashingPkScript,
		stakerPk,
		unbondingTime,
		params.UnbondingFeeSat,
		params.SlashingRate,
		regtestParams,
	)
	if err != nil {
		return err
	}

	stakingSlashingPath, err := stakingInfo.SlashingPathSpendInfo()
	if err != nil {
		return err
	}

	unbondingSlashing, err := staking.BuildSlashingTxFromStakingTxStrict(
		unbondingTx,
		0,
		params.SlashingPkScript,
		stakerPk,
		unbondingTime,
		params.UnbondingFeeSat,
		params.SlashingRate,
		regtestParams,
	)
	if err != nil {
		return err
	}
	unbondingSlashingPath, err := unbondingInfo.SlashingPathSpendInfo()
	if err != nil {
		return err
	}

	signStakingSlashingRes, err := s.SignOneInputTaprootSpendingTransaction(stakerPk, &TaprootSigningRequest{
		FundingOutput: stakingTx.TxOut[0],
		TxToSign:      stakingSlashing,
		SpendDescription: &SpendPathDescription{
			ControlBlock: &stakingSlashingPath.ControlBlock,
			ScriptLeaf:   &stakingSlashingPath.RevealedLeaf,
		},
	})
	if err != nil {
		return err
	}

	signUnbondingSlashingRes, err := s.SignOneInputTaprootSpendingTransaction(stakerPk, &TaprootSigningRequest{
		FundingOutput: unbondingTx.TxOut[0],
		TxToSign:      unbondingSlashing,
		SpendDescription: &SpendPathDescription{
			ControlBlock: &unbondingSlashingPath.ControlBlock,
			ScriptLeaf:   &unbondingSlashingPath.RevealedLeaf,
		},
	})
	if err != nil {
		return err
	}

	stakingSlashingTx, err := bbntypes.SerializeBTCTx(stakingSlashing)
	if err != nil {
		return err
	}
	stakingSlashingSig := bbntypes.NewBIP340SignatureFromBTCSig(signStakingSlashingRes.Signature)
	unbondingSlashingTx, err := bbntypes.SerializeBTCTx(unbondingSlashing)
	if err != nil {
		return err
	}
	unbondingSlashingSig := bbntypes.NewBIP340SignatureFromBTCSig(signUnbondingSlashingRes.Signature)

	pop, err := s.signBip322NativeSegwit(stakerAddress)
	if err != nil {
		return err
	}

	msgBTCDel := &bstypes.MsgCreateBTCDelegation{
		StakerAddr:              s.client.BabylonAddress.String(),
		Pop:                     pop,
		BtcPk:                   bbntypes.NewBIP340PubKeyFromBTCPK(stakerPk),
		FpBtcPkList:             []bbntypes.BIP340PubKey{*bbntypes.NewBIP340PubKeyFromBTCPK(s.fpPK)},
		StakingTime:             params.MaxStakingTimeBlocks,
		StakingValue:            params.MinStakingValueSat,
		StakingTx:               serializedStakingTx,
		StakingTxInclusionProof: inclusionProof,
		SlashingTx:              bstypes.NewBtcSlashingTxFromBytes(stakingSlashingTx),
		DelegatorSlashingSig:    stakingSlashingSig,
		// Unbonding related data
		UnbondingTime:                 uint32(unbondingTime),
		UnbondingTx:                   serializedUnbondingTx,
		UnbondingValue:                unbondingTxValue,
		UnbondingSlashingTx:           bstypes.NewBtcSlashingTxFromBytes(unbondingSlashingTx),
		DelegatorUnbondingSlashingSig: unbondingSlashingSig,
	}

	resp, err := s.client.SendMsgs(context.Background(), []sdk.Msg{msgBTCDel})
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("resp from send msg is nil %v", msgBTCDel)
	}

	fmt.Printf("Delegation sent for transaction with hash %s\n", hash)

	return nil
}

func bbnPksToBtcPks(pks []bbn.BIP340PubKey) ([]*btcec.PublicKey, error) {
	btcPks := make([]*btcec.PublicKey, 0, len(pks))
	for _, pk := range pks {
		btcPk, err := pk.ToBTCPK()
		if err != nil {
			return nil, err
		}
		btcPks = append(btcPks, btcPk)
	}
	return btcPks, nil
}