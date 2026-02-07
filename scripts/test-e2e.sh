#!/bin/bash
set -e

# E2E test script: starts the LangDAG server with mock provider,
# runs all SDK E2E tests, then cleans up.

PORT=${LANGDAG_TEST_PORT:-18090}
DB_PATH="/tmp/langdag-e2e-$$.db"
SERVER_PID=""

cleanup() {
    echo ""
    echo "Cleaning up..."
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -f "$DB_PATH" "${DB_PATH}-shm" "${DB_PATH}-wal"
    echo "Done."
}

trap cleanup EXIT

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "=== Building LangDAG server ==="
go build -o bin/langdag ./cmd/langdag

echo ""
echo "=== Starting LangDAG server with mock provider on port $PORT ==="
LANGDAG_PROVIDER=mock LANGDAG_STORAGE_PATH="$DB_PATH" \
    bin/langdag serve --port "$PORT" --host 127.0.0.1 &
SERVER_PID=$!

# Wait for server to be ready
for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:$PORT/health" > /dev/null 2>&1; then
        echo "Server ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Server failed to start"
        exit 1
    fi
    sleep 0.5
done

export LANGDAG_E2E_URL="http://127.0.0.1:$PORT"
FAILED=0

echo ""
echo "=== Running Go SDK E2E tests ==="
if (cd sdks/go && go test -v -run TestE2E ./...); then
    echo "Go SDK E2E: PASSED"
else
    echo "Go SDK E2E: FAILED"
    FAILED=1
fi

echo ""
echo "=== Running Python SDK E2E tests ==="
if [ -d sdks/python/.venv ]; then
    PYTEST="sdks/python/.venv/bin/pytest"
else
    PYTEST="pytest"
fi
if (cd sdks/python && $PYTEST tests/test_e2e.py -v); then
    echo "Python SDK E2E: PASSED"
else
    echo "Python SDK E2E: FAILED"
    FAILED=1
fi

echo ""
echo "=== Running TypeScript SDK E2E tests ==="
if (cd sdks/typescript && npx vitest run src/e2e.test.ts); then
    echo "TypeScript SDK E2E: PASSED"
else
    echo "TypeScript SDK E2E: FAILED"
    FAILED=1
fi

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "=== All E2E tests PASSED ==="
else
    echo "=== Some E2E tests FAILED ==="
    exit 1
fi
