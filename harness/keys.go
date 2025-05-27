package harness

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
)

type Key struct {
	PubKey  string `json:"pubkey"`
	PrivKey string `json:"privkey"`
	Address string `json:"address"`
}

type KeyExport struct {
	BabylonKey Key `json:"babylon_key"`
	BitcoinKey Key `json:"bitcoin_key"`
}

func GenerateAndSaveKeys(keyName string) (*KeyExport, error) {
	privKey, pubKey, address := testdata.KeyTestPubAddr()

	btcPrivKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	wif, err := btcutil.NewWIF(btcPrivKey, &chaincfg.RegressionNetParams, true)
	if err != nil {
		return nil, err
	}

	btcPubKey := btcPrivKey.PubKey()
	btcAddress, err := btcutil.NewAddressPubKey(btcPubKey.SerializeCompressed(), &chaincfg.RegressionNetParams)
	if err != nil {
		return nil, err
	}

	bbnKey := Key{
		PubKey:  hex.EncodeToString(pubKey.Bytes()),
		PrivKey: hex.EncodeToString(privKey.Bytes()),
		Address: address.String(),
	}

	btcKey := Key{
		PubKey:  hex.EncodeToString(btcPubKey.SerializeCompressed()),
		PrivKey: wif.String(),
		Address: btcAddress.EncodeAddress(),
	}

	combinedKeys := KeyExport{
		BabylonKey: bbnKey,
		BitcoinKey: btcKey,
	}

	data, err := json.MarshalIndent(combinedKeys, "", " ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshall combined keys: %w", err)
	}
	filename := fmt.Sprintf("%s.export.json", keyName)

	err = os.WriteFile(filename, data, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to write to file: %w", err)
	}

	fmt.Println("Keys generated and saved to", keyName+".export.json")
	fmt.Println("Babylon key:", bbnKey.Address)
	fmt.Println("Bitcoin key:", btcKey.Address)

	return &combinedKeys, nil
}

func LoadKeys(path string) (*KeyExport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var combinedKeys KeyExport
	err = json.Unmarshal(data, &combinedKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal from json: %w", err)
	}

	fmt.Println("Keys loaded from", path)
	fmt.Println("Babylon key:", combinedKeys.BabylonKey.Address)
	fmt.Println("Bitcoin key:", combinedKeys.BitcoinKey.Address)

	return &combinedKeys, nil
}
