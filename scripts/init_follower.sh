#!/bin/bash

echo "Initializing Follower Node..."

# Use sudo to update package lists and install packages
apt-get update
apt-get install -y iputils-ping net-tools curl netcat-openbsd dnsutils vim

# Initialize Babylon on follower node
babylond init test --chain-id euphrates-0.4.0 --home ~/.babylond

# Copy the genesis file
cp /snapshots/genesis.json ~/.babylond/config/genesis.json

# Set addr_book_strict to false in config.toml
sed -i 's/addr_book_strict = true/addr_book_strict = false/' ~/.babylond/config/config.toml

# Fetch the master node's ID
MASTER_NODE_ID=$(curl -s http://master-node:26657/status | jq -r .result.node_info.id)

if [ -z "$MASTER_NODE_ID" ]; then
    echo "Failed to fetch master node ID. Make sure the master node is running and accessible."
    exit 1
fi

# Add master node to persistent_peers in config.toml
sed -i "s/persistent_peers = \"\"/persistent_peers = \"$MASTER_NODE_ID@master-node:26656\"/" ~/.babylond/config/config.toml

echo "Follower Node Initialized with updated configuration."
echo "Master Node ID: $MASTER_NODE_ID"
echo "persistent_peers set to: $MASTER_NODE_ID@master-node:26656"
echo "addr_book_strict set to false"