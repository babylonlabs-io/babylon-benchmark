#!/bin/bash -eux

echo "Initializing Master Node with Snapshot at /snapshots/euphrates_snap.tar.gz..."

# Initialize Babylon on master node
babylond init test --chain-id euphrates-0.4.0 --home ~/.babylond

# Extract the snapshot within the container
tar -xvf /snapshots/euphrates_snap.tar.gz -C ~/.babylond --overwrite

cp /snapshots/genesis.json ~/.babylond/config/genesis.json

#babylond start --x-crisis-skip-assert-invariants --home ~/.babylond

echo "Master Node Initialized with Snapshot."