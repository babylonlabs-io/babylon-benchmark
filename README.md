# babylon-benchmark

This repository contains tools and scripts for benchmarking and profiling the Babylon blockchain node.

## Getting Started

Follow these steps to set up and run the benchmark:

1. Clone the repository:
   ```
   git clone git@github.com:babylonlabs-io/babylon-benchmark.git
   cd babylon-benchmark
   ```

2. Initialize, update, and checkout the correct version of submodules:
   ```bash
   git submodule init
   git submodule update
   cd submodules/babylon
   git checkout v0.11.0
   cd ../..
   ```
   
   Note: By default, the submodules are pointing to version `v0.11.0`, which corresponds to the Phase 2 devnet-1 snapshot. Always ensure that the Babylon version matches the snapshot you're using. You can verify the correct version in the [devnet-k8s repository](https://github.com/babylonlabs-io/devnet-k8s/blob/3cb993b2bac9ed9b88a4e92f09c0e4d1d65cad08/flux/services/phase-2/network/rpcs/helmrelease.yaml#L25). If you're using a different snapshot or version, make sure to checkout the appropriate tag or commit in the Babylon submodule.

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

   The profiler will run until follower node has synced with the master. After completion, the profile data will be automatically saved in the `outputs/` folder.

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