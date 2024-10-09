#!/bin/bash

echo "Initializing Follower Node..."

# Update package lists
apt-get update

# Install essential network and system monitoring tools
apt-get install -y \
    net-tools \
    iputils-ping \
    curl \
    wget \
    netcat-openbsd \
    dnsutils \
    tcpdump \
    iproute2 \
    iftop \
    procps \
    htop \
    iotop \
    sysstat \
    lsof \
    jq

# Initialize Babylon on follower node
babylond init test --chain-id euphrates-0.4.0 --home /root/.babylond

# Fetch the genesis file from remote RPC
curl https://rpc-euphrates.devnet.babylonchain.io/genesis | jq '.result.genesis' > /root/.babylond/config/genesis.json

# Copy the genesis file
#cp /snapshots/genesis.json /root/.babylond/config/genesis.json

# Set addr_book_strict to false in config.toml
sed -i 's/addr_book_strict = true/addr_book_strict = false/' /root/.babylond/config/config.toml

# Modify the config to listen on all interfaces
sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' /root/.babylond/config/config.toml

sed -i 's/^external_address = ""/external_address = "tcp:\/\/0.0.0.0:26656"/' /root/.babylond/config/config.toml

# Change the network to signet in app.toml
sed -i 's/network = "mainnet"/network = "signet"/' /root/.babylond/config/app.toml

# Fetch the master node's ID
MASTER_NODE_ID=$(curl -s http://master-node:26657/status | jq -r .result.node_info.id)

if [ -z "$MASTER_NODE_ID" ]; then
    echo "Failed to fetch master node ID. Make sure the master node is running and accessible."
    exit 1
fi

# Add master node to persistent_peers in config.toml
sed -i "s/persistent_peers = \"\"/persistent_peers = \"$MASTER_NODE_ID@master-node:26656\"/" /root/.babylond/config/config.toml

# Increase timeout_commit to 30s in config.toml
sed -i 's/timeout_commit = "5s"/timeout_commit = "30s"/' /root/.babylond/config/config.toml

echo "Follower Node Initialized with updated configuration."
echo "Master Node ID: $MASTER_NODE_ID"
echo "persistent_peers set to: $MASTER_NODE_ID@master-node:26656"
echo "addr_book_strict set to false"