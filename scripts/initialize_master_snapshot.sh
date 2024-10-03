#!/bin/bash -eux

SNAPSHOT_PATH=$1

echo "Initializing Master Node with Snapshot at ${SNAPSHOT_PATH}..."

# Initialize Babylon on master node
babylond init test --chain-id euphrates-0.4.0 --home /babylondhome

# Copy snapshot and genesis file into the container
# Assumes genesis.json is located at ./data/master/genesis.json
cp /snapshots/euphrates_snap.tar.gz /babylondhome/
cp /scripts/genesis.json /babylondhome/config/

# Extract the snapshot within the container
tar -xvf /babylondhome/euphrates_snap.tar.gz -C /babylondhome/ --overwrite

echo "Master Node Initialized with Snapshot."