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


# Load genesis from a local file or fetch from RPC as before
if [ -n "$GENESIS_PATH" ]; then
    echo "Loading genesis file from $GENESIS_PATH"
    if [ -f "$GENESIS_PATH" ]; then
        GENESIS_TEMP=$(jq '.' "$GENESIS_PATH")
    else
        echo "Error: File not found at $GENESIS_PATH"
        exit 1
    fi
else
    echo "Fetching genesis file from $RPC_URL"
    GENESIS_TEMP=$(curl -s "$RPC_URL/genesis" | jq '.result.genesis')
fi

if [ -z "$GENESIS_TEMP" ]; then
    echo "Error: Failed to load genesis"
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

# Extract the snapshot within the container
echo "Extracting snapshot $SNAPSHOT_FILE..."
tar -xvf /snapshots/$SNAPSHOT_FILE -C /root/.babylond --overwrite

# Now that /root/.babylond exists, we can save the genesis file
echo "$GENESIS_TEMP" > /root/.babylond/config/genesis.json

# Modify the config to listen on all interfaces
sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' /root/.babylond/config/config.toml

sed -i 's/^external_address = ""/external_address = "tcp:\/\/0.0.0.0:26656"/' /root/.babylond/config/config.toml

# Change the network to signet in app.toml
sed -i 's/network = "mainnet"/network = "signet"/' /root/.babylond/config/app.toml

# Increase timeout_commit to 30s in config.toml
sed -i 's/timeout_commit = "5s"/timeout_commit = "30s"/' /root/.babylond/config/config.toml

# Enable metrics
sed -i 's/prometheus = false/prometheus = true/' /root/.babylond/config/config.toml

# Disable iavl cache otherwise OOM
sed -i 's/iavl-cache-size = 781250/iavl-cache-size = 0/' /root/.babylond/config/app.toml
sed -i 's/iavl-disable-fastnode = false/iavl-disable-fastnode = true/' /root/.babylond/config/app.toml

echo "Master Node Initialized with Snapshot."