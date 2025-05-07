package harness

import (
	"context"
	"fmt"
	"time"

	staking "github.com/babylonlabs-io/babylon/btcstaking"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type CovenantEmulator struct {
	tm     *TestManager
	client *SenderWithBabylonClient
	covKey *btcec.PrivateKey
}

func NewCovenantEmulator(
	tm *TestManager,
	covKey *btcec.PrivateKey,
	client *SenderWithBabylonClient,
) *CovenantEmulator {
	return &CovenantEmulator{
		tm:     tm,
		client: client,
		covKey: covKey,
	}
}

func (c *CovenantEmulator) Start(ctx context.Context) {
	go c.runForever(ctx)
}

func (c *CovenantEmulator) runForever(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendMsgsWithSig(ctx); err != nil {
				fmt.Printf("🚫 Err sending cov msgs %v\n", err)
			}
		}
	}
}

func (c *CovenantEmulator) sendMsgsWithSig(ctx context.Context) error {
	params, err := c.client.BTCStakingParams()
	if err != nil {
		return err
	}
	respo, err := c.client.BTCDelegations(bstypes.BTCDelegationStatus_PENDING, nil)
	if err != nil {
		return err
	}
	if respo == nil {
		return fmt.Errorf("err getting btc delegation in cov")
	}

	if len(respo.BtcDelegations) == 0 {
		return nil
	}

	messages, err := c.messagesWithSignatures(respo.BtcDelegations, &params.Params)
	if err != nil {
		return err
	}

	if err := c.client.SendMsgs(ctx, messages); err != nil {
		return err
	}

	return nil
}

func (c *CovenantEmulator) covenantSignatures(
	fpEncKey *asig.EncryptionKey,
	stakingSlashingTx *wire.MsgTx,
	stakingTx *wire.MsgTx,
	stakingOutputIdx uint32,
	stakingUnbondingScript []byte,
	stakingSlashingScript []byte,
	unbondingTx *wire.MsgTx,
	slashUnbondingTx *wire.MsgTx,
	unbondingSlashingScript []byte,
) (slashSig, slashUnbondingSig *asig.AdaptorSignature, unbondingSig *schnorr.Signature, err error) {
	// creates slash sigs
	slashSig, err = staking.EncSignTxWithOneScriptSpendInputStrict(
		stakingSlashingTx,
		stakingTx,
		stakingOutputIdx,
		stakingSlashingScript,
		c.covKey,
		fpEncKey,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	// creates slash unbonding sig
	slashUnbondingSig, err = staking.EncSignTxWithOneScriptSpendInputStrict(
		slashUnbondingTx,
		unbondingTx,
		0, // 0th output is always the unbonding script output
		unbondingSlashingScript,
		c.covKey,
		fpEncKey,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	unbondingSig, err = staking.SignTxWithOneScriptSpendInputStrict(
		unbondingTx,
		stakingTx,
		stakingOutputIdx,
		stakingUnbondingScript,
		c.covKey,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return slashSig, slashUnbondingSig, unbondingSig, nil
}

func (c *CovenantEmulator) messagesWithSignatures(resp []*bstypes.BTCDelegationResponse, params *bstypes.Params) ([]sdk.Msg, error) {
	var msgs []sdk.Msg

	for _, del := range resp {
		stakingTx, _, err := bbntypes.NewBTCTxFromHex(del.StakingTxHex)
		if err != nil {
			return nil, err
		}

		stakingSlashingTx, _, err := bbntypes.NewBTCTxFromHex(del.SlashingTxHex)
		if err != nil {
			return nil, err
		}

		unbondingTx, _, err := bbntypes.NewBTCTxFromHex(del.UndelegationResponse.UnbondingTxHex)
		if err != nil {
			return nil, err
		}

		slashUnbondingTx, _, err := bbntypes.NewBTCTxFromHex(del.UndelegationResponse.SlashingTxHex)
		if err != nil {
			return nil, err
		}

		stakerPk := del.BtcPk.MustToBTCPK()
		fpPk := del.FpBtcPkList[0].MustToBTCPK()
		stakingTime := del.EndHeight - del.StartHeight
		covenatKeys, err := bbnPksToBtcPks(params.CovenantPks)
		if err != nil {
			return nil, err
		}

		fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(fpPk)
		if err != nil {
			return nil, err
		}

		stakingInfo, err := staking.BuildStakingInfo(
			stakerPk,
			[]*btcec.PublicKey{fpPk},
			covenatKeys,
			params.CovenantQuorum,
			uint16(stakingTime),
			btcutil.Amount(del.TotalSat),
			regtestParams,
		)
		if err != nil {
			return nil, err
		}

		unbondingInfo, err := staking.BuildUnbondingInfo(
			stakerPk,
			[]*btcec.PublicKey{fpPk},
			covenatKeys,
			params.CovenantQuorum,
			uint16(del.UnbondingTime),
			btcutil.Amount(unbondingTx.TxOut[0].Value),
			regtestParams,
		)
		if err != nil {
			return nil, err
		}

		stakingSlahingPath, err := stakingInfo.SlashingPathSpendInfo()
		if err != nil {
			return nil, err
		}

		stakingUnbondingPath, err := stakingInfo.UnbondingPathSpendInfo()
		if err != nil {
			return nil, err
		}

		unbondingSlashingPath, err := unbondingInfo.SlashingPathSpendInfo()
		if err != nil {
			return nil, err
		}

		stakingSlashingSig, unbondingSlashingSig, unbondingSig, err := c.covenantSignatures(
			fpEncKey,
			stakingSlashingTx,
			stakingTx,
			del.StakingOutputIdx,
			stakingUnbondingPath.RevealedLeaf.Script,
			stakingSlahingPath.RevealedLeaf.Script,
			unbondingTx,
			slashUnbondingTx,
			unbondingSlashingPath.RevealedLeaf.Script,
		)
		if err != nil {
			return nil, err
		}

		stakingTxHash := stakingTx.TxHash()
		msg := &bstypes.MsgAddCovenantSigs{
			Signer:                  c.client.BabylonAddress.String(),
			Pk:                      bbntypes.NewBIP340PubKeyFromBTCPK(c.covKey.PubKey()),
			StakingTxHash:           stakingTxHash.String(),
			SlashingTxSigs:          [][]byte{stakingSlashingSig.MustMarshal()},
			UnbondingTxSig:          bbntypes.NewBIP340SignatureFromBTCSig(unbondingSig),
			SlashingUnbondingTxSigs: [][]byte{unbondingSlashingSig.MustMarshal()},
		}

		msgs = append(msgs, msg)
	}

	return msgs, nil
}
