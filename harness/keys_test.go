package harness_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon-benchmark/harness"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func TestLoadKeys(t *testing.T) {
	keyName := "test"
	filename := fmt.Sprintf("%s.export.json", keyName)

	t.Cleanup(func() {
		os.Remove(filename)
	})

	keys, err := harness.GenerateAndSaveKeys(keyName)
	if err != nil {
		t.Fatalf("failed to generate and save keys %v", err)
	}

	btcPubKeyBefore, err := hex.DecodeString(keys.BitcoinKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode btc pub  key string into bytes %v", err)
	}

	wif, err := btcutil.DecodeWIF(keys.BitcoinKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode  btc priv key string into bytes %v", err)
	}

	btcPrivKeyBefore := wif.PrivKey.Key.Bytes()

	btcAddressBefore, err := btcutil.DecodeAddress(keys.BitcoinKey.Address, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatalf("failed to decode btc address string into bytes %v", err)
	}

	bbnPubBefore, err := hex.DecodeString(keys.BabylonKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode bbn pubkey string into bytes %v", err)
	}

	bbnPrivBefore, err := hex.DecodeString(keys.BabylonKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode bbn privkey string into bytes %v", err)
	}

	bbnAddressBefore, err := hex.DecodeString(keys.BabylonKey.Address)
	if err != nil {
		t.Fatalf("failed to decode bbn address string into bytes %v", err)
	}

	loadedkeys, err := harness.LoadKeys(filename)
	if err != nil {
		t.Fatalf("failed to load keys %v", err)
	}

	bbnPubKeyBytes, err := hex.DecodeString(loadedkeys.BabylonKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode babylon public key %v", err)
	}

	bbnPrivKeyBytes, err := hex.DecodeString(loadedkeys.BabylonKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode babylon private key %v", err)
	}

	bbnAddressBytes, err := hex.DecodeString(loadedkeys.BabylonKey.Address)
	if err != nil {
		t.Fatalf("failed to decode babylon private key %v", err)
	}

	btcPubKeybytes, err := hex.DecodeString(loadedkeys.BitcoinKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode btc public key %v", err)
	}

	PrivKeyBytes, err := btcutil.DecodeWIF(loadedkeys.BitcoinKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode btc private key %v", err)
	}

	privKeyBytes := PrivKeyBytes.PrivKey.Key.Bytes()

	address, err := btcutil.DecodeAddress(loadedkeys.BitcoinKey.Address, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatalf("failed to decode bitcoin address %v", err)
	}

	btcAddressBytes := address.EncodeAddress()

	if !bytes.Equal(btcPubKeybytes, btcPubKeyBefore) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", string(btcPubKeybytes), btcPubKeyBefore)
	}

	if !bytes.Equal(privKeyBytes[:], btcPrivKeyBefore[:]) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", string(btcPubKeybytes), btcPrivKeyBefore)
	}

	if !bytes.Equal([]byte(btcAddressBytes), []byte(btcAddressBefore.EncodeAddress())) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", []byte(btcAddressBytes), btcAddressBefore)
	}

	if !bytes.Equal(bbnAddressBefore, bbnAddressBytes) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", bbnAddressBefore, bbnAddressBytes)
	}

	if !bytes.Equal(bbnPrivBefore, bbnPrivKeyBytes) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", bbnPrivBefore, bbnPrivKeyBytes)
	}

	if !bytes.Equal(bbnPubBefore, bbnPubKeyBytes) {
		t.Fatalf("original bitcoin key %v and loaded bitcoin key %v are not equal", bbnPubBefore, bbnPubKeyBytes)
	}
}
