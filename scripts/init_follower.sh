#!/bin/bash -eux

echo "Initializing Master Node with Snapshot at /snapshots/euphrates_snap.tar.gz..."

# Initialize Babylon on master node
babylond init test --chain-id euphrates-0.4.0 --home ~/.babylond

# Extract the snapshot within the container
#tar -xvf /snapshots/euphrates_snap.tar.gz -C ~/.babylond --overwrite

cp /snapshots/genesis.json ~/.babylond/config/genesis.json

# Set addr_book_strict to false in config.toml
sed -i 's/addr_book_strict = true/addr_book_strict = false/' ~/.babylond/config/config.toml

# Fetch the master node's ID
MASTER_NODE_ID=$(curl -s http://master-node:26657/status | jq -r .result.node_info.id)

# Add the master node as a persistent peer
sed -i "s/persistent_peers = \"\"/persistent_peers = \"$MASTER_NODE_ID@master-node:26656\"/" ~/.babylond/config/config.toml

sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' ~/.babylond/config/config.toml

echo "Follower Node Initialized."