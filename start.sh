#!/bin/bash

# Trap SIGINT and SIGTERM to kill all background processes on exit
cleanup() {
  echo ""
  echo "Stopping all development servers..."
  kill "$VITE_PID" 2>/dev/null
  kill "$GO_PID" 2>/dev/null
  exit 0
}
trap cleanup SIGINT SIGTERM EXIT

# Load environment variables from .env if it exists
if [ -f .env ]; then
  # export env variables excluding comments
  export $(grep -v '^#' .env | xargs)
else
  echo "Warning: .env file not found. Copying .env.example to .env..."
  cp .env.example .env
  echo "Please configure your .env file with GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, and ENCRYPTION_KEY!"
fi

# Start frontend dev server in background (in a clean subshell)
echo "Starting Vite frontend dev server..."
(cd web && npm run dev) &
VITE_PID=$!

# Function to get current hash of all Go files and .env
get_go_hash() {
  find cmd internal -name "*.go" -type f -exec stat -f "%m %N" {} + 2>/dev/null | md5
}

# Start Go backend server
start_go() {
  echo "Starting Go API server..."
  go run ./cmd/server &
  GO_PID=$!
}

# Initial start
start_go
LAST_HASH=$(get_go_hash)

echo "--------------------------------------------------------"
echo "Development environment is ready!"
echo "- Frontend is running at: http://localhost:${WEB_PORT:-5175}"
echo "- Backend API is running at: http://localhost:8080"
echo "- Watching Go files for auto-restart..."
echo "--------------------------------------------------------"

# Watch loop
while true; do
  sleep 2
  CURRENT_HASH=$(get_go_hash)
  if [ "$CURRENT_HASH" != "$LAST_HASH" ]; then
    echo ""
    echo "--- Go source code changes detected. Rebuilding and restarting Go API server... ---"
    kill "$GO_PID" 2>/dev/null
    wait "$GO_PID" 2>/dev/null
    start_go
    LAST_HASH="$CURRENT_HASH"
  fi
done
