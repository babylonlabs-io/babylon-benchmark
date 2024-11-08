#!/bin/bash

# Check if RPC_URL is set
if [ -z "$RPC_URL" ]; then
    echo "Error: RPC_URL environment variable is not set."
    echo "Please specify the RPC URL by setting the RPC_URL environment variable."
    exit 1
fi

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

# Fetch the genesis file from remote RPC and store it temporarily
echo "Fetching genesis file from $RPC_URL"
GENESIS_TEMP=$(curl -s "$RPC_URL/genesis" | jq '.result.genesis')
if [ -z "$GENESIS_TEMP" ]; then
    echo "Error: Failed to fetch genesis file"
    exit 1
fi

# Extract chain_id from the temporary genesis data
CHAIN_ID=$(echo "$GENESIS_TEMP" | jq -r '.chain_id')

# Verify that CHAIN_ID was successfully extracted
if [ -z "$CHAIN_ID" ]; then
    echo "Error: Failed to extract chain_id from genesis file"
    exit 1
fi

echo "Extracted chain_id from genesis: $CHAIN_ID"

# Initialize Babylon on master node
echo "Initializing Babylon with chain ID: $CHAIN_ID"
babylond init test --chain-id $CHAIN_ID --home /root/.babylond

# Now that /root/.babylond exists, we can save the genesis file
echo "$GENESIS_TEMP" > /root/.babylond/config/genesis.json

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

# Change pprof_laddr to 0.0.0.0:6060
sed -i 's/^pprof_laddr = "localhost:6060"/pprof_laddr = "0.0.0.0:6060"/' /root/.babylond/config/config.toml

# Disable iavl cache otherwise OOM
sed -i 's/iavl-cache-size = 781250/iavl-cache-size = 0/' /root/.babylond/config/app.toml
sed -i 's/iavl-disable-fastnode = false/iavl-disable-fastnode = true/' /root/.babylond/config/app.toml

echo "Follower Node Initialized with updated configuration."
echo "Master Node ID: $MASTER_NODE_ID"
echo "persistent_peers set to: $MASTER_NODE_ID@master-node:26656"
echo "addr_book_strict set to false"