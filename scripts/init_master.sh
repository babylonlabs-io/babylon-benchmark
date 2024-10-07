#!/bin/bash -eux

echo "Initializing Master Node..."

# Use sudo to update package lists and install packages
apt-get update
apt-get install -y iputils-ping net-tools curl netcat-openbsd dnsutils vim

# Initialize Babylon on master node
babylond init test --chain-id euphrates-0.4.0 --home ~/.babylond

# Extract the snapshot within the container
tar -xvf /snapshots/euphrates_snap.tar.gz -C ~/.babylond --overwrite

cp /snapshots/genesis.json ~/.babylond/config/genesis.json

# Modify the config to listen on all interfaces
sed -i 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' ~/.babylond/config/config.toml

sed -i 's/^external_address = ""/external_address = "tcp:\/\/0.0.0.0:26657"/' ~/.babylond/config/config.toml

echo "Master Node Initialized with Snapshot."