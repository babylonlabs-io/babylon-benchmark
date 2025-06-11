package cmd

import (
	"fmt"

	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/harness"
	"github.com/spf13/cobra"
)

const (
	totalFpFlag          = "total-fps"
	totalStakersFlag     = "total-stakers"
	babylonPathFlag      = "babylon-path"
	totalDelegationsFlag = "total-delegations"
	numPubRandFlag       = "num-public-randomness"
	iavlDisabledFastnode = "iavl-disabled-fastnode"
	iavlCacheSize        = "iavl-cache-size"
	numMatureOutputsFlag = "num-mature-outputs"
	keyName              = "key-name"
	BabylonAddress       = "babylon-address"
	grpcaddr             = "grpc-address"
	bbngrpcaddr          = "bbn-grpc-address"
	bbnrpcaddr           = "bbn-rpc-address"
	btcgrpcaddr          = "btc-grpc-addr"
	btcrpcaddr           = "btc-rpc-addr"
	btcpass              = "btc-pass"
	btcuser              = "btcuser"
	remotenode           = "remote-node"
	pathToKeyExport      = "path-to-key-export"
)

// CommandGenerate generates data
func CommandGenerate() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generates delegations with configurable finality providers, stakers, and total delegations",
		Example: `dgd generate --babylon-path /path/to/babylon --total-fps 5 --total-stakers 150 --total-delegations 500`,
		Args:    cobra.NoArgs,
		RunE:    cmdGenerate,
	}

	f := cmd.Flags()
	f.String(babylonPathFlag, "", "Path to which babylond docker will mount (optional, tmp dir will be used as default)")
	f.Int(totalFpFlag, 3, "Number of finality providers to run (optional)")
	f.Int(totalDelegationsFlag, 0, "Number of delegations to run this cmd, after it we will exit. (optional, 0 for indefinite)")
	f.Int(totalStakersFlag, 100, "Number of stakers to run (optional)")
	f.Uint32(numPubRandFlag, 150_000, "Number of pub randomness to commit, should be a high value (optional)")
	f.Bool(iavlDisabledFastnode, true, "IAVL disabled fast node (additional fast node cache) (optional)")
	f.Uint(iavlCacheSize, 0, "IAVL cache size, note cache too big can cause OOM, 100k -> ~20 GB of RAM (optional)")
	f.Uint32(numMatureOutputsFlag, 4000, "Number of blocks to be mined")

	return cmd
}

func CommandGenerateAndSaveKey() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "gen-key",
		Aliases: []string{"gsk"},
		Short:   "Generates new Bitcoin and Babylon keys and saves them to a file for remote node funding",
		Example: `dgd gen-key --key-name my-key`,
		Args:    cobra.NoArgs,
		RunE:    cmdGenerateAndSaveKeys,
	}

	f := cmd.Flags()
	f.String(keyName, "", "Name of the key to generate")
	if err := cmd.MarkFlagRequired(keyName); err != nil {
		panic(err)
	}

	return cmd
}

func CommandGenerateRemote() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "generate remote",
		Aliases: []string{"gr"},
		Short:   "Generates delegations with remote babylon and bitcoin nodes",
		Example: `dgd generate-remote --babylon-path /path/to/babylon --bbn-rpc-addr http://localhost:26657 --bbn-grpc-addr http://localhost:9090 --btc-rpc-addr http://localhost:8332 --btc-grpc-addr http://localhost:11000 --home /path/to/keys`,
		Args:    cobra.NoArgs,
		RunE:    cmdGenerateRemote,
	}

	f := cmd.Flags()
	f.String(btcpass, "", "Bitcoin RPC password")
	if err := cmd.MarkFlagRequired(btcpass); err != nil {
		panic(err)
	}

	f.String(btcuser, "", "Bitcoin RPC user")
	if err := cmd.MarkFlagRequired(btcuser); err != nil {
		panic(err)
	}

	f.String(btcgrpcaddr, "", "Bitcoin gRPC address")
	if err := cmd.MarkFlagRequired(btcgrpcaddr); err != nil {
		panic(err)
	}

	f.String(btcrpcaddr, "", "Bitcoin RPC address")
	if err := cmd.MarkFlagRequired(btcrpcaddr); err != nil {
		panic(err)
	}

	f.String(bbngrpcaddr, "", "Babylon gRPC address")
	if err := cmd.MarkFlagRequired(bbngrpcaddr); err != nil {
		panic(err)
	}

	f.String(bbnrpcaddr, "", "Babylon RPC address")
	if err := cmd.MarkFlagRequired(bbnrpcaddr); err != nil {
		panic(err)
	}

	f.String(pathToKeyExport, "", "File path to key export")
	if err := cmd.MarkFlagRequired(pathToKeyExport); err != nil {
		panic(err)
	}

	return cmd
}

func cmdGenerateRemote(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	btcrpcaddr, err := flags.GetString(btcrpcaddr)
	if err != nil {
		return fmt.Errorf("failed to read flag %s", btcrpcaddr)
	}

	btcgrpcaddr, err := flags.GetString(btcgrpcaddr)
	if err != nil {
		return fmt.Errorf("failed to read flag %s", btcgrpcaddr)
	}

	btcuser, err := flags.GetString(btcuser)
	if err != nil {
		return fmt.Errorf("failed to read flag: %s", btcuser)
	}

	btcpass, err := flags.GetString(btcpass)
	if err != nil {
		return fmt.Errorf("failed to read flag: %s", btcpass)
	}

	bbngrpcaddr, err := flags.GetString(bbngrpcaddr)
	if err != nil {
		return fmt.Errorf("failed to read flag: %s", bbngrpcaddr)
	}

	bbnrpcaddr, err := flags.GetString(bbnrpcaddr)
	if err != nil {
		return fmt.Errorf("failed to read flag: %s", bbnrpcaddr)
	}

	cfg := config.Config{
		BTCRPC:          btcrpcaddr,
		BTCGRPC:         btcgrpcaddr,
		BTCPass:         btcpass,
		BTCUser:         btcuser,
		BBNGRPC:         bbngrpcaddr,
		BBNRPC:          bbnrpcaddr,
		PathToKeyExport: pathToKeyExport,
	}

	if err := cfg.ValidateRemote(); err != nil {
		return err
	}

	// harness.RunRemote(cmd.Context(), cfg)

	return nil
}

func cmdGenerate(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	babylonPath, err := flags.GetString(babylonPathFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", babylonPathFlag, err)
	}

	totalFps, err := flags.GetInt(totalFpFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalFpFlag, err)
	}

	totalDelegations, err := flags.GetInt(totalDelegationsFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalDelegationsFlag, err)
	}

	totalStakers, err := flags.GetInt(totalStakersFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalStakersFlag, err)
	}

	numPubRand, err := flags.GetUint32(numPubRandFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", numPubRandFlag, err)
	}

	iavlCache, err := flags.GetUint(iavlCacheSize)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", iavlCacheSize, err)
	}

	disabledFastnode, err := flags.GetBool(iavlDisabledFastnode)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", iavlDisabledFastnode, err)
	}

	numMatureOutputs, err := flags.GetUint32(numMatureOutputsFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", numMatureOutputsFlag, err)
	}

	cfg := config.Config{
		NumPubRand:             numPubRand,
		TotalStakers:           totalStakers,
		TotalFinalityProviders: totalFps,
		TotalDelegations:       totalDelegations,
		BabylonPath:            babylonPath,
		IavlCacheSize:          iavlCache,
		IavlDisableFastnode:    disabledFastnode,
		NumMatureOutputs:       numMatureOutputs,
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	return harness.Run(cmd.Context(), cfg)
}

func cmdGenerateAndSaveKeys(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	keyName, err := flags.GetString(keyName)

	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", keyName, err)
	}

	keys, err := harness.GenerateAndSaveKeys(keyName)
	if err != nil {
		return err
	}

	fmt.Printf("Keys generated 💅 and saved to %s\nBabylon key 🔑 %s\nBitcoin key 🔑 %s\n", keyName+".export.json", keys.BabylonKey.Address, keys.BitcoinKey.Address)

	return nil
}
