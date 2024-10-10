#!/bin/bash -eux

# Check if SNAPSHOT_FILE is set
if [ -z "$SNAPSHOT_FILE" ]; then
    echo "Error: SNAPSHOT_FILE environment variable is not set."
    echo "Please specify the snapshot file by setting the SNAPSHOT_FILE environment variable."
    exit 1
fi

# Check if the snapshot file exists
if [ ! -f "/snapshots/$SNAPSHOT_FILE" ]; then
    echo "Error: Snapshot file /snapshots/$SNAPSHOT_FILE not found."
    echo "Please ensure you've placed the correct snapshot file in the snapshots/ directory."
    exit 1
fi

# Check if CHAIN_ID is set
if [ -z "$CHAIN_ID" ]; then
    echo "Error: CHAIN_ID environment variable is not set."
    echo "Please specify the chain ID by setting the CHAIN_ID environment variable."
    exit 1
fi

# Check if RPC_URL is set
if [ -z "$RPC_URL" ]; then
    echo "Error: RPC_URL environment variable is not set."
    echo "Please specify the RPC URL by setting the RPC_URL environment variable."
    exit 1
fi

echo "Initializing Master Node..."

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


# Initialize Babylon on master node
echo "Initializing Babylon with chain ID: $CHAIN_ID"
babylond init test --chain-id $CHAIN_ID --home /root/.babylond

# Extract the snapshot within the container
echo "Extracting snapshot $SNAPSHOT_FILE..."
tar -xvf /snapshots/$SNAPSHOT_FILE -C /root/.babylond --overwrite

# Fetch the genesis file from remote RPC
echo "Fetching genesis file from $RPC_URL"
curl "$RPC_URL/genesis" | jq '.result.genesis' > /root/.babylond/config/genesis.json

# Modify the config to listen on all interfaces
sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' /root/.babylond/config/config.toml

sed -i 's/^external_address = ""/external_address = "tcp:\/\/0.0.0.0:26656"/' /root/.babylond/config/config.toml

# Change the network to signet in app.toml
sed -i 's/network = "mainnet"/network = "signet"/' /root/.babylond/config/app.toml

# Increase timeout_commit to 30s in config.toml
sed -i 's/timeout_commit = "5s"/timeout_commit = "30s"/' /root/.babylond/config/config.toml

echo "Master Node Initialized with Snapshot."