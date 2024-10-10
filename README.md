# babylon-benchmark

This repository contains tools and scripts for benchmarking and profiling the Babylon blockchain node.

## Getting Started

Follow these steps to set up and run the benchmark:

1. Clone the repository:
   ```
   git clone git@github.com:babylonlabs-io/babylon-benchmark.git
   cd babylon-benchmark
   ```

2. Initialize and update submodules:
   ```
   git submodule init && git submodule update
   ```

3. Prepare the snapshot:
   - Download the Phase2 devnet snapshot from [this thread](https://babylonlabsworkspace.slack.com/archives/G07DYV8MA1M/p1728476907088119?thread_ts=1728461084.441899&cid=G07DYV8MA1M).
   - Place the downloaded `.tar.gz` file in the `snapshots/` folder of your project.
   - The extraction and booting process will be automatically handled by the program. You only need to ensure the `.tar.gz` file is present in the `snapshots/` directory.
   - Update the snapshot path in `scripts/init_master.sh` if necessary:
     ```shell
     # Edit this line in scripts/init_master.sh to match your snapshot filename
     tar -xvf /snapshots/your_snapshot_name.tar.gz -C /root/.babylond --overwrite
     ```

4. Start the benchmark:
   ```
   make start-benchmark-from-snapshot
   ```
   This will build the docker images and start both the master and follower nodes. The follower node will begin syncing with the master.

5. Run the profiler:
   Once the follower node has synced with the master, you can start profiling:
   ```
   ./scripts/run-profiler.sh
   ```
   After completion, the profile data will be saved under `outputs/` folder.

6. Stop the nodes:
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