package harness

import (
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
	m *container.Manager
}

func NewBitcoindHandler(manager *container.Manager) *BitcoindTestHandler {
	return &BitcoindTestHandler{
		m: manager,
	}
}

func (h *BitcoindTestHandler) Start(containerName string) (*dockertest.Resource, error) {
	// todo(lazar): cleanup
	tempPath, err := os.MkdirTemp("", "bbn-benchmark-test-*")
	if err != nil {
		return nil, err
	}

	//h.t.Cleanup(func() {
	//	_ = os.RemoveAll(tempPath)
	//})

	bitcoinResource, err := h.m.RunBitcoindResource(containerName, tempPath)
	if err != nil {
		return nil, err
	}

	//h.t.Cleanup(func() {
	//	_ = h.m.ClearResources()
	//})

	err = lib.Eventually(func() bool {
		_, err = h.GetBlockCount()
		if err != nil {
			fmt.Printf("failed to get block count: %v", err)
		}
		return err == nil
	}, startTimeout, 500*time.Millisecond)

	if err != nil {
		return nil, err
	}

	return bitcoinResource, nil
}

// GetBlockCount retrieves the current number of blocks in the blockchain from the Bitcoind.
func (h *BitcoindTestHandler) GetBlockCount() (int, error) {
	buff, _, err := h.m.ExecBitcoindCliCmd([]string{"getblockcount"})
	if err != nil {
		return 0, err
	}

	parsedBuffStr := strings.TrimSuffix(buff.String(), "\n")

	return strconv.Atoi(parsedBuffStr)
}

// GenerateBlocks mines a specified number of blocks in the Bitcoind.
func (h *BitcoindTestHandler) GenerateBlocks(count int) *GenerateBlockResponse {
	buff, _, err := h.m.ExecBitcoindCliCmd([]string{"-generate", fmt.Sprintf("%d", count)})
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
func (h *BitcoindTestHandler) CreateWallet(walletName string, passphrase string) *CreateWalletResponse {
	// last arg is true which indicates we are using descriptor wallets they do not allow dumping private keys.
	buff, _, err := h.m.ExecBitcoindCliCmd([]string{"createwallet", walletName, "false", "false", passphrase, "false", "true"})
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
func (h *BitcoindTestHandler) InvalidateBlock(blockHash string) {
	_, _, err := h.m.ExecBitcoindCliCmd([]string{"invalidateblock", blockHash})
	if err != nil {
		panic(err)
	}
}

// ImportDescriptors imports a given Bitcoin address descriptor into the Bitcoind
func (h *BitcoindTestHandler) ImportDescriptors(descriptor string) {
	_, _, err := h.m.ExecBitcoindCliCmd([]string{"importdescriptors", descriptor})
	if err != nil {
		panic(err)
	}
}

func (h *BitcoindTestHandler) Stop() {
	_ = h.m.ClearResources()
}
