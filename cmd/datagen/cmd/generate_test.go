package cmd_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon-benchmark/cmd/datagen/cmd"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func TestLoadKeys(t *testing.T) {
	keyName := "test"

	t.Cleanup(func() {
		os.Remove(keyName + ".export.json")
	})

	err := cmd.GenerateAndSaveKeys(keyName)
	if err != nil {
		t.Fatalf("failed to generate and save keys %v", err)
	}

	os.Stat(keyName + ".export.json")
	if err != nil {
		t.Fatalf("failed to describe file %v", err)
	}

	key, err := cmd.LoadKeys(keyName + ".export.json")
	if err != nil {
		t.Fatalf("failed to load keys %v", err)
	}

	_, err = hex.DecodeString(key.BabylonKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode babylon public key %v", err)
	}

	_, err = hex.DecodeString(key.BabylonKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode babylon private key %v", err)
	}

	bitCoinPubKeybytes, err := hex.DecodeString(key.BitcoinKey.PubKey)
	if err != nil {
		t.Fatalf("failed to decode btc public key %v", err)
	}

	btcPrivKeyBytes, err := btcutil.DecodeWIF(key.BitcoinKey.PrivKey)
	if err != nil {
		t.Fatalf("failed to decode btc private key %v", err)
	}

	if !bytes.Equal(btcPrivKeyBytes.PrivKey.PubKey().SerializeCompressed(), bitCoinPubKeybytes) {
		t.Fatalf("Bitcoin pub and private keys dont match %v", err)
	}

	_, err = btcutil.DecodeAddress(key.BitcoinKey.Address, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatalf("failed to decode bitcoin address %v", err)
	}

	if key.BabylonKey.Address == "" || key.BabylonKey.PrivKey == "" || key.BabylonKey.PubKey == "" {
		t.Fatalf("Missing Babylon keys required")
	}

	if key.BitcoinKey.Address == "" || key.BitcoinKey.PrivKey == "" || key.BitcoinKey.PubKey == "" {
		t.Fatalf("Missing Bitcoin keys required")
	}
}
