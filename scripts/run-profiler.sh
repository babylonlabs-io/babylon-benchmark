#!/bin/bash

# Function to get block height
get_height() {
    docker exec $1 babylond status | jq -r '.sync_info.latest_block_height'
}

# Start profiling
start_profiling() {
    echo "Starting profiler..."
    curl -o cpu.pprof http://localhost:6061/debug/pprof/profile &
    PROFILER_PID=$!
}

# Stop profiling
stop_profiling() {
    echo "Stopping profiler..."
    kill $PROFILER_PID
}

# Start profiling
start_profiling

# Monitor sync progress
while true; do
    MASTER_HEIGHT=$(get_height master-node)
    FOLLOWER_HEIGHT=$(get_height follower-node)
    
    echo "Master height: $MASTER_HEIGHT, Follower height: $FOLLOWER_HEIGHT"
    
    # Check if heights are close (within 5 blocks)
    if [ $((MASTER_HEIGHT - FOLLOWER_HEIGHT)) -le 5 ]; then
        echo "Sync complete!"
        stop_profiling
        break
    fi
    
    sleep 60  # Check every minute
done

echo "Profiling complete. Results saved in cpu.pprof"