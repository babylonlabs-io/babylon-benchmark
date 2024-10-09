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
   - Place your desired snapshot in the `snapshots/` folder.
   - Update the snapshot path in `scripts/init_master.sh`:
     ```shell
     # Edit this line in scripts/init_master.sh
     tar -xvf /snapshots/your_snapshot_name.tar.gz -C /root/.babylond --overwrite
     ```

4. Start the benchmark:
   ```
   ./start-benchmark-from-snapshot
   ```
   This will start both the master and follower nodes. The follower node will begin syncing with the master.

5. Run the profiler:
   Once the follower node has synced with the master, you can start profiling:
   ```
   ./scripts/run-profiler.sh
   ```
   After completion, the profile data will be saved in `outputs/cpu.pprof`.

## Directory Structure

- `snapshots/`: Place your blockchain snapshots here.
- `scripts/`: Contains initialization and profiling scripts.
- `outputs/`: Profiling output files are stored here.

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