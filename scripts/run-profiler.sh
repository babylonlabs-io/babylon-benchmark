#!/bin/bash

PROFILE_FILE=""

# Function to get block height
get_height() {
    height=$(docker exec $1 babylond status 2>/dev/null | jq -r '.sync_info.latest_block_height' 2>/dev/null)
    if [[ "$height" =~ ^[0-9]+$ ]]; then
        echo "$height"
    else
        echo "error"
    fi
}

# Function to check if a container is running
container_is_running() {
    if [ "$(docker inspect -f '{{.State.Running}}' $1 2>/dev/null)" != "true" ]; then
        echo "Error: Container $1 is not running."
        return 1
    fi
    return 0
}

# Start profiling
start_profiling() {
    PROFILE_FILE="outputs/profile_$(date +%Y%m%d_%H%M%S).pprof"
    curl -o "$PROFILE_FILE" http://localhost:6061/debug/pprof/profile?seconds=60
    echo "Profiling complete. Profile saved to $PROFILE_FILE"
}

# Check if both containers are running
if ! container_is_running master-node || ! container_is_running follower-node; then
    echo "Error: One or both required containers (master-node, follower-node) are not running."
    exit 1
fi

# Wait for follower node to start syncing
PROFILER_STARTED=false
while true; do
    MASTER_HEIGHT=$(get_height master-node)
    FOLLOWER_HEIGHT=$(get_height follower-node)
    
    # Check if heights are valid numbers
    if [ "$MASTER_HEIGHT" = "error" ] || [ "$FOLLOWER_HEIGHT" = "error" ]; then
        echo "Waiting for master and follower to initialize, retrying in 5 seconds..."
        sleep 5
        continue
    fi

    echo "Master height: $MASTER_HEIGHT, Follower height: $FOLLOWER_HEIGHT"

    if [ "$FOLLOWER_HEIGHT" -gt 1 ] && [ "$PROFILER_STARTED" = false ]; then
        echo "Follower node has started syncing. Starting profiler..."
        start_profiling
        PROFILER_STARTED=true
        break
    fi

    sleep 5  # Check every 5 seconds
done

echo "Profiling complete"