#!/bin/sh
set -e # Exit immediately if a command exits with a non-zero status.

# Start Redis server in the background
echo "Starting Redis server..."
redis-server --daemonize yes --port 6379 --bind 127.0.0.1

# Wait for Redis to start
echo "Waiting for Redis to be ready..."
RETRY_COUNT=0
MAX_RETRIES=60 # Wait for max 60 seconds
until redis-cli -h 127.0.0.1 -p 6379 ping | grep -q PONG; do
  RETRY_COUNT=$((RETRY_COUNT + 1))
  if [ ${RETRY_COUNT} -gt ${MAX_RETRIES} ]; then
    echo "Redis failed to start after ${MAX_RETRIES} seconds."
    exit 1
  fi
  sleep 1
done
echo "Redis is ready."

export GOOGLE_APPLICATION_CREDENTIALS=/app/lazervault-c4d5f9e91078.json

# Execute the main application (passed as CMD arguments)
echo "Starting Go application: $@"
exec "$@" 