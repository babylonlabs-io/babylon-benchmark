# babylon-benchmark

This repository contains tools and scripts for benchmarking and profiling the Babylon blockchain node.

## Getting Started

The repository consists of two independent parts, data generation CLI (dgd) and scripts used to demonstrate sync from a 
snapshot.

### The data generation CLI (dgd)

Data generation CLI is built from part of Babylon auxiliary programs (vigilante, btc-staker, finality provider) and 
is able to create activated delegations and send finality votes, basically simulating what auxiliary programs are doing.

It starts a bitcoind, node0 and node1 docker containers, where node0 and node1 are Babylon validator nodes, 
node0 is used for all other RPC related queries while node1 is strictly used for submitting delegations.

#### Building the CLI

1. Build the CLI by running `make build`.
2. Confirm that `dgd` CLI has been built successfully by running `./build/dgd version`

#### Command examples 

The following command starts:
- 20 finality providers
- 200 btc stakers
- total-delegations - indicates that the daemon will shut down after this number

```shell
./build/dgd generate --total-fps 20 --total-stakers 200 --total-delegations 1250000 --babylon-path /data/babylon-benchmark
```

Sample log from the dgd:
```
🔎 Found 1 non-finalized block(s). Next block to finalize: 2723
📌 Sending checkpoint for epoch 141 with proof 2
📄 Delegations sent: 22585, rate: 8.00 delegations/sec, ts: Fri Dec  6 15:58:16 UTC 2024, mem: 5347 MB
⏱️ Average delegation submission time: 2.0138 seconds
✍️ Fp e5342f02b71cabec2ef96d8adf1d4d1c98706e3f928911b00b74fed763b98363, voted for block range [2723-2723]
✍️ Fp 63bc13483ff1b3b436ec8a1a4d8c542e4b63fee4f3e3e19e0e7cf3b583b06fee, voted for block range [2723-2723]
```

### Starting main & follower nodes from a snapshot

Follow these steps to set up and run the benchmark:

1. Clone the repository:
   ```
   git clone git@github.com:babylonlabs-io/babylon-benchmark.git
   cd babylon-benchmark
   ```

2. Initialize, update, and check out the correct version of submodules:
   ```bash
   git submodule init
   git submodule update
   cd submodules/babylon
   git checkout v0.18.0
   cd ../..
   ```
   
Note: By default, the submodules are pointing to version `v0.18.0`, which corresponds to the Phase 2 devnet-1 snapshot. 
Always ensure that the Babylon version matches the snapshot you're using. You can verify the correct version in the [devnet-k8s repository](https://github.com/babylonlabs-io/devnet-k8s/blob/3cb993b2bac9ed9b88a4e92f09c0e4d1d65cad08/flux/services/phase-2/network/rpcs/helmrelease.yaml#L25). 
If you're using a different snapshot or version, make sure to checkout the appropriate tag or commit in the Babylon submodule.

3. Prepare the snapshot:
   - Download the Phase 2 devnet snapshot from [this thread](https://babylonlabsworkspace.slack.com/archives/G07DYV8MA1M/p1728476907088119?thread_ts=1728461084.441899&cid=G07DYV8MA1M).
   - Place the downloaded `.tar.gz` file in the `snapshots/` folder of your project.
   - The extraction and booting process will be automatically handled by the program. You only need to ensure the `.tar.gz` file is present in the `snapshots/` directory.
   - Update the `SNAPSHOT_FILE` environment variable in your `docker-compose.yml` to match your snapshot filename:
     ```yaml
     environment:
       - SNAPSHOT_FILE=your_snapshot_name.tar.gz
     ```

4. Start the benchmark:
   ```
   make start-benchmark-from-snapshot
   ```

   This command will:
   - Build the Docker images
   - Start both the master and follower nodes
   - Bootstrap the master node with the provided snapshot
   - Start the follower node which will begin syncing with the master
   - Start the profiler once the follower node has started syncing

   The profiler will by default run for 60 seconds. To modify the profiling duration:
   - Open `scripts/run-profiler.sh`
   - Locate the `start_profiling` function
   - Change the `seconds=60` parameter in the curl command to your desired duration

   For example, to profile for 120 seconds, you would modify the line to:
   ```
   curl -o "$PROFILE_FILE" http://localhost:6061/debug/pprof/profile?seconds=120
   ```

   After completion, the profile data will be automatically saved in the `outputs/` folder. You can visualize the profile data using the following command:
    ```
    go tool pprof -http=:8080 outputs/profile_<timestamp>.pprof
    ```

5. Stop the nodes:
   When you're done with the benchmark, you can stop the nodes using:
   ```
   make stop-benchmark
   ```
   This will shut down both the master and follower nodes and clean up the data directories.

## Directory Structure

- `snapshots/`: Place your blockchain snapshots here.
- `scripts/`: Contains initialization and profiling scripts.
- `outputs/`: Contains the profile data.

## Notes

- Ensure you have sufficient disk space for the blockchain data and snapshots.
- The profiler will automatically stop once the follower node has synced with the master.
- Review the `scripts/init_master.sh` and `scripts/init_follower.sh` for any additional configuration needed.

## Troubleshooting

If you encounter any issues:
- Check that the snapshot path in `init_master.sh` is correct.
- Ensure Docker is installed and running on your system.
- Verify that the required ports are not in use by other applications.

For more detailed information, refer to the individual script files.