package container

import (
	"bytes"
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	"regexp"
	"strconv"
	"time"

	bbn "github.com/babylonlabs-io/babylon/types"

	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

const (
	bitcoindContainerName = "bitcoind"
	babylondContainerName = "babylond"
)

var (
	CovenantPrivKey, _ = btcec.NewPrivateKey()
	CovenantPubKey     = CovenantPrivKey.PubKey()
)

var errRegex = regexp.MustCompile(`(E|e)rror`)

// Manager is a wrapper around all Docker instances, and the Docker API.
// It provides utilities to run and interact with all Docker containers
type Manager struct {
	cfg       ImageConfig
	pool      *dockertest.Pool
	resources map[string]*dockertest.Resource
}

// NewManager creates a new Manager instance and initializes
// all Docker specific utilities. Returns an error if initialization fails.
func NewManager() (docker *Manager, err error) {
	imgCfg, err := NewImageConfig()
	if err != nil {
		return nil, err
	}
	docker = &Manager{
		cfg:       *imgCfg,
		resources: make(map[string]*dockertest.Resource),
	}
	docker.pool, err = dockertest.NewPool("")
	if err != nil {
		return nil, err
	}

	return docker, nil
}

func (m *Manager) ExecBitcoindCliCmd(command []string) (bytes.Buffer, bytes.Buffer, error) {
	// this is currently hardcoded, as it will be the same for all tests
	cmd := []string{"bitcoin-cli", "-chain=regtest", "-rpcuser=user", "-rpcpassword=pass"}
	cmd = append(cmd, command...)
	return m.ExecCmd(bitcoindContainerName, cmd)
}

// ExecCmd executes command by running it on the given container.
// It word for word `error` in output to discern between error and regular output.
// It returns stdout and stderr as bytes.Buffer and an error if the command fails.
func (m *Manager) ExecCmd(containerName string, command []string) (bytes.Buffer, bytes.Buffer, error) {
	if _, ok := m.resources[containerName]; !ok {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("no resource %s found", containerName)
	}
	containerId := m.resources[containerName].Container.ID

	var (
		outBuf bytes.Buffer
		errBuf bytes.Buffer
	)

	timeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// We use the `require.Eventually` function because it is only allowed to do one transaction per block without
	// sequence numbers. For simplicity, we avoid keeping track of the sequence number and just use the `require.Eventually`.
	var innerErr error
	err := lib.Eventually(
		func() bool {
			exec, err := m.pool.Client.CreateExec(docker.CreateExecOptions{
				Context:      ctx,
				AttachStdout: true,
				AttachStderr: true,
				Container:    containerId,
				User:         "root",
				Cmd:          command,
			})

			if err != nil {
				innerErr = fmt.Errorf("failed to create exec: %v", err)
				return false
			}

			err = m.pool.Client.StartExec(exec.ID, docker.StartExecOptions{
				Context:      ctx,
				Detach:       false,
				OutputStream: &outBuf,
				ErrorStream:  &errBuf,
			})
			if err != nil {
				innerErr = fmt.Errorf("failed to create exec: %v", err)

				return false
			}

			errBufString := errBuf.String()
			// Note that this does not match all errors.
			// This only works if CLI outputs "Error" or "error"
			// to stderr.
			if errRegex.MatchString(errBufString) {
				innerErr = fmt.Errorf("failed to create exec: %v", err)
				return false
			}

			return true
		},
		timeout,
		500*time.Millisecond,
	)

	if err != nil {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("docker cmd failed %v %v", err, innerErr)
	}

	return outBuf, errBuf, nil
}

// RunBitcoindResource starts a bitcoind docker container
func (m *Manager) RunBitcoindResource(
	name string,
	bitcoindCfgPath string,
) (*dockertest.Resource, error) {
	bitcoindResource, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       fmt.Sprintf("%s-%s", bitcoindContainerName, name),
			Repository: m.cfg.BitcoindRepository,
			Tag:        m.cfg.BitcoindVersion,
			User:       "root:root",
			Labels: map[string]string{
				"e2e": "bitcoind",
			},
			Mounts: []string{
				fmt.Sprintf("%s/:/data/.bitcoin", bitcoindCfgPath),
			},
			ExposedPorts: []string{
				"18443/tcp",
			},
			Cmd: []string{
				"-regtest",
				"-txindex",
				"-rpcuser=user",
				"-rpcpassword=pass",
				"-rpcallowip=0.0.0.0/0",
				"-rpcbind=0.0.0.0",
				"-zmqpubsequence=tcp://0.0.0.0:28333",
				"-fallbackfee=0.0002",
			},
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"18443/tcp": {{HostIP: "", HostPort: strconv.Itoa(AllocateUniquePort())}}, // only expose what we need
			}
			config.PublishAllPorts = false // because in dockerfile they already expose them
		},
		noRestart,
	)
	if err != nil {
		return nil, err
	}
	m.resources[bitcoindContainerName] = bitcoindResource
	return bitcoindResource, nil
}

// RunBabylondResource starts a babylond container
func (m *Manager) RunBabylondResource(
	name string,
	mounthPath string,
	baseHeaderHex string,
	slashingPkScript string,
	epochInterval uint,
) (*dockertest.Resource, error) {
	cmd := []string{
		"sh", "-c", fmt.Sprintf(
			"babylond testnet --v=1 --output-dir=/home --starting-ip-address=192.168.10.2 "+
				"--keyring-backend=test --chain-id=chain-test --btc-finalization-timeout=4 "+
				"--btc-confirmation-depth=2 --additional-sender-account --btc-network=regtest "+
				"--min-staking-time-blocks=200 --min-staking-amount-sat=10000 "+
				"--epoch-interval=%d --slashing-pk-script=%s --btc-base-header=%s "+
				"--covenant-quorum=1 --covenant-pks=%s && chmod -R 777 /home && babylond start --home=/home/node0/babylond",
			epochInterval, slashingPkScript, baseHeaderHex, bbn.NewBIP340PubKeyFromBTCPK(CovenantPubKey).MarshalHex()),
	}

	resource, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       fmt.Sprintf("%s-%s", babylondContainerName, name),
			Repository: m.cfg.BabylonRepository,
			Tag:        m.cfg.BabylonVersion,
			Labels: map[string]string{
				"e2e": "babylond",
			},
			User: "root:root",
			Mounts: []string{
				fmt.Sprintf("%s/:/home/", mounthPath),
			},
			ExposedPorts: []string{
				"9090/tcp", // only expose what we need
				"26657/tcp",
			},
			Cmd: cmd,
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"9090/tcp":  {{HostIP: "", HostPort: strconv.Itoa(AllocateUniquePort())}},
				"26657/tcp": {{HostIP: "", HostPort: strconv.Itoa(AllocateUniquePort())}},
			}
		},
		noRestart,
	)
	if err != nil {
		return nil, err
	}

	m.resources[babylondContainerName] = resource

	return resource, nil
}

// ClearResources removes all outstanding Docker resources created by the Manager.
func (m *Manager) ClearResources() error {
	for _, resource := range m.resources {
		if err := m.pool.Purge(resource); err != nil {
			return err
		}
	}

	return nil
}

func noRestart(config *docker.HostConfig) {
	// in this case, we don't want the nodes to restart on failure
	config.RestartPolicy = docker.RestartPolicy{
		Name: "no",
	}
}
