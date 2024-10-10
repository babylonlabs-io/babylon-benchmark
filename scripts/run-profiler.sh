#!/bin/bash

# Function to get block height
get_height() {
    docker exec $1 babylond status | jq -r '.sync_info.latest_block_height'
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
    echo "Starting profiler..."
    curl -o "outputs/profile_$(date +%Y%m%d_%H%M%S).pprof" http://localhost:6061/debug/pprof/profile &
    PROFILER_PID=$!
}

# Stop profiling
stop_profiling() {
    echo "Stopping profiler..."
    kill $PROFILER_PID
}

# Check if both containers are running
if ! container_is_running master-node || ! container_is_running follower-node; then
    echo "Error: One or both required containers (master-node, follower-node) are not running."
    exit 1
fi

# Start profiling
start_profiling

# Monitor sync progress
while true; do
    MASTER_HEIGHT=$(get_height master-node)
    FOLLOWER_HEIGHT=$(get_height follower-node)
    
    # Check if heights are valid numbers
    if ! [[ "$MASTER_HEIGHT" =~ ^[0-9]+$ ]] || ! [[ "$FOLLOWER_HEIGHT" =~ ^[0-9]+$ ]]; then
        echo "Invalid heights. Master: $MASTER_HEIGHT, Follower: $FOLLOWER_HEIGHT"
        echo "Retrying in 60 seconds..."
        sleep 60
        continue
    fi

    echo "Master height: $MASTER_HEIGHT, Follower height: $FOLLOWER_HEIGHT"

    # Check if heights are close (within 5 blocks)
    if [ $((MASTER_HEIGHT - FOLLOWER_HEIGHT)) -le 5 ]; then
        echo "Sync complete!"
        stop_profiling
        break
    fi
    
    sleep 60  # Check every minute
done

echo "Profiling complete"