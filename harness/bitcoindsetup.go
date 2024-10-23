package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	"github.com/ory/dockertest/v3"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	startTimeout = 30 * time.Second
)

type CreateWalletResponse struct {
	Name    string `json:"name"`
	Warning string `json:"warning"`
}

type GenerateBlockResponse struct {
	Address string   `json:"address"`
	Blocks  []string `json:"blocks"`
}

type BitcoindTestHandler struct {
	m    *container.Manager
	path string
}

func NewBitcoindHandler(manager *container.Manager) *BitcoindTestHandler {
	return &BitcoindTestHandler{
		m: manager,
	}
}

func (h *BitcoindTestHandler) Start(ctx context.Context, containerName string) (*dockertest.Resource, error) {
	tempPath, err := os.MkdirTemp("", "bbn-benchmark-test-*")
	if err != nil {
		return nil, err
	}

	bitcoinResource, err := h.m.RunBitcoindResource(containerName, tempPath)
	if err != nil {
		return nil, err
	}

	err = lib.Eventually(ctx, func() bool {
		_, err := h.GetBlockCount(ctx)

		if err != nil {
			fmt.Printf("🚫 Failed to get block count: %v\n", err)
			return false
		}
		return err == nil
	}, startTimeout, 500*time.Millisecond, "err waiting for block count")

	if err != nil {
		return nil, err
	}

	h.path = tempPath

	return bitcoinResource, nil
}

// GetBlockCount retrieves the current number of blocks in the blockchain from the Bitcoind.
func (h *BitcoindTestHandler) GetBlockCount(ctx context.Context) (int, error) {
	buff, _, err := h.m.ExecBitcoindCliCmd(ctx, []string{"getblockcount"})
	if err != nil {
		return 0, err
	}

	parsedBuffStr := strings.TrimSuffix(buff.String(), "\n")

	return strconv.Atoi(parsedBuffStr)
}

// GenerateBlocks mines a specified number of blocks in the Bitcoind.
func (h *BitcoindTestHandler) GenerateBlocks(ctx context.Context, count int) *GenerateBlockResponse {
	buff, _, err := h.m.ExecBitcoindCliCmd(ctx, []string{"-generate", fmt.Sprintf("%d", count)})
	if err != nil {
		panic(err)
	}

	var response GenerateBlockResponse
	err = json.Unmarshal(buff.Bytes(), &response)
	if err != nil {
		panic(err)
	}

	return &response
}

// CreateWallet creates a new wallet with the specified name and passphrase in the Bitcoind
func (h *BitcoindTestHandler) CreateWallet(ctx context.Context, walletName string, passphrase string) *CreateWalletResponse {
	// last arg is true which indicates we are using descriptor wallets they do not allow dumping private keys.
	buff, _, err := h.m.ExecBitcoindCliCmd(ctx, []string{"createwallet", walletName, "false", "false", passphrase, "false", "true"})
	if err != nil {
		panic(err)
	}

	var response CreateWalletResponse
	err = json.Unmarshal(buff.Bytes(), &response)
	if err != nil {
		panic(err)
	}

	return &response
}

// InvalidateBlock invalidates blocks starting from specified block hash
func (h *BitcoindTestHandler) InvalidateBlock(ctx context.Context, blockHash string) {
	_, _, err := h.m.ExecBitcoindCliCmd(ctx, []string{"invalidateblock", blockHash})
	if err != nil {
		panic(err)
	}
}

// ImportDescriptors imports a given Bitcoin address descriptor into the Bitcoind
func (h *BitcoindTestHandler) ImportDescriptors(ctx context.Context, descriptor string) {
	_, _, err := h.m.ExecBitcoindCliCmd(ctx, []string{"importdescriptors", descriptor})
	if err != nil {
		panic(err)
	}
}

func (h *BitcoindTestHandler) Stop() {
	cleanupDir(h.path)
}
