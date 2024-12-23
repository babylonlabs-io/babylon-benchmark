package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	"github.com/docker/docker/api/types"
	"regexp"
	"strconv"
	"time"

	bbn "github.com/babylonlabs-io/babylon/types"
	rawDc "github.com/docker/docker/client"

	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

const (
	bitcoindContainerName      = "bitcoind"
	babylondNode0ContainerName = "babylond-node0"
	babylondNode1ContainerName = "babylond-node1"
)

var (
	CovenantPrivKey, _ = btcec.NewPrivateKey()
	CovenantPubKey     = CovenantPrivKey.PubKey()
)

var errRegex = regexp.MustCompile(`(E|e)rror`)

// Manager is a wrapper around all Docker instances, and the Docker API.
// It provides utilities to run and interact with all Docker containers
type Manager struct {
	cfg         ImageConfig
	pool        *dockertest.Pool
	resources   map[string]*dockertest.Resource
	rawDcClient *rawDc.Client
	network     *docker.Network
}

// NewManager creates a new Manager instance and initializes
// all Docker specific utilities. Returns an error if initialization fails.
func NewManager() (mgr *Manager, err error) {
	imgCfg, err := NewImageConfig()
	if err != nil {
		return nil, err
	}
	mgr = &Manager{
		cfg:       *imgCfg,
		resources: make(map[string]*dockertest.Resource),
	}
	mgr.pool, err = dockertest.NewPool("")
	if err != nil {
		return nil, err
	}

	dc, err := rawDc.NewClientWithOpts(rawDc.FromEnv)
	if err != nil {
		return nil, err
	}

	mgr.rawDcClient = dc

	return mgr, nil
}

func (m *Manager) ExecBitcoindCliCmd(ctx context.Context, command []string) (bytes.Buffer, bytes.Buffer, error) {
	// this is currently hardcoded, as it will be the same for all tests
	cmd := []string{"bitcoin-cli", "-chain=regtest", "-rpcuser=user", "-rpcpassword=pass"}
	cmd = append(cmd, command...)
	return m.ExecCmd(ctx, bitcoindContainerName, cmd)
}

// ExecCmd executes command by running it on the given container.
// It word for word `error` in output to discern between error and regular output.
// It returns stdout and stderr as bytes.Buffer and an error if the command fails.
func (m *Manager) ExecCmd(ctx context.Context, containerName string, command []string) (bytes.Buffer, bytes.Buffer, error) {
	if _, ok := m.resources[containerName]; !ok {
		return bytes.Buffer{}, bytes.Buffer{}, fmt.Errorf("no resource %s found", containerName)
	}
	containerId := m.resources[containerName].Container.ID

	var (
		outBuf bytes.Buffer
		errBuf bytes.Buffer
	)

	timeout := 5 * time.Second
	innerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// We use the `require.Eventually` function because it is only allowed to do one transaction per block without
	// sequence numbers. For simplicity, we avoid keeping track of the sequence number and just use the `require.Eventually`.
	var innerErr error
	err := lib.Eventually(
		innerCtx,
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
	bitcoindCfgPath string,
) (*dockertest.Resource, error) {
	bitcoindResource, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       bitcoindContainerName,
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
	mounthPath string,
	baseHeaderHex string,
	slashingPkScript string,
	epochInterval uint,
	iavlDisableFastnode bool,
	iavlCacheSize uint,
) (*dockertest.Resource, *dockertest.Resource, error) {
	network, err := m.pool.Client.CreateNetwork(docker.CreateNetworkOptions{
		Name:   "babylon",
		Driver: "bridge",
		IPAM: &docker.IPAMOptions{
			Config: []docker.IPAMConfig{
				{
					Subnet: "192.168.10.0/24",
				},
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	cmd := []string{
		"sh", "-c", fmt.Sprintf(
			"babylond testnet --v=2 --output-dir=/home --starting-ip-address=192.168.10.2 "+
				"--keyring-backend=test --chain-id=chain-test --btc-finalization-timeout=4 "+
				"--btc-confirmation-depth=2 --additional-sender-account --btc-network=regtest "+
				"--min-staking-time-blocks=200 --min-staking-amount-sat=10000 "+
				"--epoch-interval=%d --slashing-pk-script=%s --btc-base-header=%s "+
				"--covenant-quorum=1 --covenant-pks=%s "+
				"--min-signed-per-window=0 && "+ // never jail sluggish fps
				"chmod -R 777 /home && "+
				"sed -i -e 's/iavl-cache-size = 0/iavl-cache-size = %d/' /home/node0/babylond/config/app.toml && "+
				"sed -i -e 's/iavl-disable-fastnode = true/iavl-disable-fastnode = %s/' /home/node0/babylond/config/app.toml && "+
				`sed -i -e 's/timeout_commit = "5s"/timeout_commit = "2s"/' /home/node0/babylond/config/config.toml &&`+
				"babylond start --home=/home/node0/babylond --rpc.pprof_laddr=0.0.0.0:6060",
			epochInterval, slashingPkScript, baseHeaderHex,
			bbn.NewBIP340PubKeyFromBTCPK(CovenantPubKey).MarshalHex(),
			iavlCacheSize,
			fmt.Sprintf("%t", iavlDisableFastnode)),
	}

	resourceFirstNode, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       babylondNode0ContainerName,
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
				"6060/tcp",
			},
			Cmd: cmd,
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"9090/tcp":  {{HostIP: "", HostPort: "9090"}},
				"26657/tcp": {{HostIP: "", HostPort: "26657"}},
				"6060/tcp":  {{HostIP: "", HostPort: "6060"}},
			}
		},
		noRestart,
	)
	if err != nil {
		return nil, nil, err
	}

	err = m.pool.Client.ConnectNetwork(network.ID, docker.NetworkConnectionOptions{
		Container: resourceFirstNode.Container.ID,
		EndpointConfig: &docker.EndpointConfig{
			IPAMConfig: &docker.EndpointIPAMConfig{
				IPv4Address: "192.168.10.2",
			},
		},
	})

	if err != nil {
		return nil, nil, err
	}

	cmd2 := []string{
		"sh", "-c", fmt.Sprintf(
			"chmod -R 777 /home && ls -la &&"+
				"sed -i -e 's/iavl-cache-size = 0/iavl-cache-size = %d/' /home/node1/babylond/config/app.toml && "+
				"sed -i -e 's/iavl-disable-fastnode = true/iavl-disable-fastnode = %s/' /home/node1/babylond/config/app.toml && "+
				`sed -i -e 's/timeout_commit = "5s"/timeout_commit = "2s"/' /home/node1/babylond/config/config.toml &&`+
				"babylond start --home=/home/node1/babylond --rpc.pprof_laddr=0.0.0.0:6060",
			iavlCacheSize,
			fmt.Sprintf("%t", iavlDisableFastnode),
		),
	}

	time.Sleep(2 * time.Second) // todo(lazar): do a query on that file path to see if testnet cmd is done

	resourceSecondNode, err := m.pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       babylondNode1ContainerName,
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
				"6060/tcp",
			},
			Cmd: cmd2,
		},
		func(config *docker.HostConfig) {
			config.PortBindings = map[docker.Port][]docker.PortBinding{
				"9090/tcp":  {{HostIP: "", HostPort: "9091"}},
				"26657/tcp": {{HostIP: "", HostPort: "26658"}},
				"6060/tcp":  {{HostIP: "", HostPort: "6061"}},
			}
		},
		noRestart,
	)
	if err != nil {
		return nil, nil, err
	}

	err = m.pool.Client.ConnectNetwork(network.ID, docker.NetworkConnectionOptions{
		Container: resourceSecondNode.Container.ID,
		EndpointConfig: &docker.EndpointConfig{
			IPAMConfig: &docker.EndpointIPAMConfig{
				IPv4Address: "192.168.10.3",
			},
		},
	})

	if err != nil {
		return nil, nil, err
	}

	m.resources[babylondNode0ContainerName] = resourceFirstNode
	m.resources[babylondNode1ContainerName] = resourceSecondNode

	m.network = network

	return resourceFirstNode, resourceSecondNode, nil
}

func (m *Manager) MemoryUsage(ctx context.Context, containerName string) (uint64, error) {
	containerId := m.resources[containerName].Container.ID

	res, err := m.rawDcClient.ContainerStats(ctx, containerId, false)
	if err != nil {
		return 0, err
	}

	defer res.Body.Close()

	// Decode stats JSON
	var containerStats types.StatsJSON //nolint:staticcheck
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&containerStats); err != nil {
		return 0, err
	}

	return containerStats.MemoryStats.Usage, nil
}

// ClearResources removes all outstanding Docker resources created by the Manager.
func (m *Manager) ClearResources() error {
	for _, resource := range m.resources {
		if err := m.pool.Purge(resource); err != nil {
			return err
		}
	}

	if err := m.pool.Client.RemoveNetwork(m.network.ID); err != nil {
		return err
	}

	return nil
}

func noRestart(config *docker.HostConfig) {
	// in this case, we don't want the nodes to restart on failure
	config.RestartPolicy = docker.RestartPolicy{
		Name: "no",
	}
}
