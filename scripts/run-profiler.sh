#!/bin/bash

PROFILE_FILE=""
PROFILER_PID=""

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

start_profiling() {
    echo "Starting profiler..."
    PROFILE_FILE="outputs/profile_$(date +%Y%m%d_%H%M%S).pprof"
    (
        while true; do
            curl -s --max-time 70 "http://localhost:6061/debug/pprof/profile?seconds=60" >> "$PROFILE_FILE"
            sleep 1
        done
    ) &
    PROFILER_PID=$!
    echo "Profiler started with PID $PROFILER_PID. Profile will be saved to $PROFILE_FILE"
}

stop_profiling() {
    echo "Stopping profiler..."
    if [ -n "$PROFILER_PID" ]; then
        kill $PROFILER_PID
        wait $PROFILER_PID 2>/dev/null
        echo "Profiler stopped. Profile saved to $PROFILE_FILE"
    else
        echo "No profiler PID found. Profiler may not have been started."
    fi
    
    if [ -s "$PROFILE_FILE" ]; then
        echo "Profile file created successfully at $PROFILE_FILE"
    else
        echo "Warning: Profile file is empty or not created at $PROFILE_FILE"
    fi
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
    if [ "$MASTER_HEIGHT" = "error" ] || [ "$FOLLOWER_HEIGHT" = "error" ]; then
        echo "Waiting for master and follower to start, retrying in 10 seconds..."
        sleep 5
        continue
    fi

    echo "Master height: $MASTER_HEIGHT, Follower height: $FOLLOWER_HEIGHT"

    # Check if heights are close (within 5 blocks)
    if [ $((MASTER_HEIGHT - FOLLOWER_HEIGHT)) -le 5 ]; then
        echo "Sync complete!"
        stop_profiling
        break
    fi
    
    sleep 5  # Check every 5 seconds
done

echo "Profiling complete"