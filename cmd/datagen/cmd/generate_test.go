package cmd_test

import (
	"os"
	"testing"

	"github.com/babylonlabs-io/babylon-benchmark/cmd/datagen/cmd"
	"github.com/test-go/testify/require"
)

func TestGenerateAndSaveKeys(t *testing.T) {
	keyName := "test"

	defer os.Remove(keyName + ".export.json")

	err := cmd.GenerateAndSaveKeys(keyName)
	require.NoError(t, err)

	os.Stat(keyName + ".export.json")
	require.NoError(t, err)
}

func TestLoadKeys(t *testing.T) {
	keyName := "test"

	defer os.Remove(keyName + ".export.json")

	err := cmd.GenerateAndSaveKeys(keyName)
	require.NoError(t, err)

	os.Stat(keyName + ".export.json")
	require.NoError(t, err)

	key, err := cmd.LoadKeys(keyName + ".export.json")
	require.NoError(t, err)

	if key.BabylonKey.Address == "" || key.BabylonKey.PrivKey == "" || key.BabylonKey.PubKey == "" {
		t.Errorf("Missing Babylon keys required")
		require.Error(t, err)
	}

	if key.BitcoinKey.Address == "" || key.BitcoinKey.PrivKey == "" || key.BitcoinKey.PubKey == "" {
		t.Errorf("Missing Bitcoin keys required")
		require.Error(t, err)
	}
}
