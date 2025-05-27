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
		t.Fatalf("failed to decode string into bytes %v", err)
	}
	fmt.Println(keys.BitcoinKey.PrivKey)
	wif, err := btcutil.DecodeWIF(keys.BitcoinKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode string into bytes %v", err)
	}

	btcPrivKeyBefore := wif.PrivKey.Key.Bytes()

	btcAddressBefore, err := btcutil.DecodeAddress(keys.BitcoinKey.Address, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatalf("failed to decode string into bytes %v", err)
	}

	loadedkeys, err := harness.LoadKeys(filename)
	if err != nil {
		t.Fatalf("failed to load keys %v", err)
	}

	_, err = hex.DecodeString(loadedkeys.BabylonKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode babylon public key %v", err)
	}

	_, err = hex.DecodeString(loadedkeys.BabylonKey.PrivKey)
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
}
