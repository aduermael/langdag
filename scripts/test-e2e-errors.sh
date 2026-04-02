#!/bin/bash
set -e

# E2E streaming error tests: starts LangDAG servers with mock error modes,
# runs all SDK streaming error tests, then cleans up.

PORT_ECHO=${LANGDAG_TEST_PORT:-18090}
PORT_ERROR=$((PORT_ECHO + 1))
PORT_STREAM_ERROR=$((PORT_ECHO + 2))

DB_ECHO="/tmp/langdag-e2e-echo-$$.db"
DB_ERROR="/tmp/langdag-e2e-error-$$.db"
DB_STREAM_ERROR="/tmp/langdag-e2e-stream-error-$$.db"

PIDS=()

cleanup() {
    echo ""
    echo "Cleaning up..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    done
    rm -f "$DB_ECHO" "${DB_ECHO}-shm" "${DB_ECHO}-wal"
    rm -f "$DB_ERROR" "${DB_ERROR}-shm" "${DB_ERROR}-wal"
    rm -f "$DB_STREAM_ERROR" "${DB_STREAM_ERROR}-shm" "${DB_STREAM_ERROR}-wal"
    echo "Done."
}

trap cleanup EXIT

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "=== Building LangDAG server ==="
go build -o bin/langdag ./cmd/langdag

wait_for_server() {
    local port=$1
    local name=$2
    for i in $(seq 1 30); do
        if curl -sf "http://127.0.0.1:$port/health" > /dev/null 2>&1; then
            echo "$name ready on port $port."
            return 0
        fi
        if [ "$i" -eq 30 ]; then
            echo "ERROR: $name failed to start on port $port"
            return 1
        fi
        sleep 0.5
    done
}

# Start echo server (normal mode for regular E2E tests)
echo ""
echo "=== Starting echo server on port $PORT_ECHO ==="
LANGDAG_PROVIDER=mock LANGDAG_MOCK_MODE=echo LANGDAG_STORAGE_PATH="$DB_ECHO" \
    bin/langdag serve --port "$PORT_ECHO" --host 127.0.0.1 &
PIDS+=($!)

# Start error server (immediate error on every request)
echo "=== Starting error server on port $PORT_ERROR ==="
LANGDAG_PROVIDER=mock LANGDAG_MOCK_MODE=error LANGDAG_MOCK_ERROR_MESSAGE="test error" \
    LANGDAG_STORAGE_PATH="$DB_ERROR" \
    bin/langdag serve --port "$PORT_ERROR" --host 127.0.0.1 &
PIDS+=($!)

# Start stream_error server (sends 3 chunks then errors)
echo "=== Starting stream_error server on port $PORT_STREAM_ERROR ==="
LANGDAG_PROVIDER=mock LANGDAG_MOCK_MODE=stream_error \
    LANGDAG_MOCK_ERROR_MESSAGE="test stream error" \
    LANGDAG_MOCK_ERROR_AFTER_CHUNKS=3 \
    LANGDAG_STORAGE_PATH="$DB_STREAM_ERROR" \
    bin/langdag serve --port "$PORT_STREAM_ERROR" --host 127.0.0.1 &
PIDS+=($!)

wait_for_server "$PORT_ECHO" "Echo server"
wait_for_server "$PORT_ERROR" "Error server"
wait_for_server "$PORT_STREAM_ERROR" "Stream error server"

export LANGDAG_E2E_URL="http://127.0.0.1:$PORT_ECHO"
export LANGDAG_E2E_ERROR_URL="http://127.0.0.1:$PORT_ERROR"
export LANGDAG_E2E_STREAM_ERROR_URL="http://127.0.0.1:$PORT_STREAM_ERROR"

FAILED=0

echo ""
echo "=== Running Go SDK E2E tests ==="
if (cd sdks/go && GOWORK=off go test -v -run TestE2E ./...); then
    echo "Go SDK E2E: PASSED"
else
    echo "Go SDK E2E: FAILED"
    FAILED=1
fi

echo ""
echo "=== Running Python SDK E2E tests ==="
if [ ! -d sdks/python/.venv ]; then
    echo "Setting up Python venv..."
    (cd sdks/python && python3 -m venv .venv && .venv/bin/pip install -e ".[dev]" -q)
fi
PYTEST="$ROOT_DIR/sdks/python/.venv/bin/pytest"
if (cd sdks/python && "$PYTEST" tests/test_e2e.py -v); then
    echo "Python SDK E2E: PASSED"
else
    echo "Python SDK E2E: FAILED"
    FAILED=1
fi

echo ""
echo "=== Running TypeScript SDK E2E tests ==="
if [ ! -d sdks/typescript/node_modules ]; then
    echo "Installing TypeScript dependencies..."
    (cd sdks/typescript && npm install -q)
fi
if (cd sdks/typescript && npx vitest run src/e2e.test.ts); then
    echo "TypeScript SDK E2E: PASSED"
else
    echo "TypeScript SDK E2E: FAILED"
    FAILED=1
fi

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "=== All E2E streaming error tests PASSED ==="
else
    echo "=== Some E2E streaming error tests FAILED ==="
    exit 1
fi
