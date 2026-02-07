.PHONY: build run mockllm mockllm-run clean \
	test test-unit test-e2e test-go test-python test-typescript

# Main LangDAG server
build:
	go build -o bin/langdag ./cmd/langdag

run: build
	./bin/langdag serve

# Mock LLM server
mockllm:
	cd tools/mockllm && go build -o ../../bin/mockllm .

mockllm-run: mockllm
	./bin/mockllm

# Testing
test: test-unit test-e2e

test-unit: test-go test-python test-typescript

test-e2e:
	./scripts/test-e2e.sh

test-go:
	cd sdks/go && go test -v ./...

test-python:
	cd sdks/python && .venv/bin/pytest tests/test_client.py tests/test_async_client.py -v

test-typescript:
	cd sdks/typescript && npx vitest run

clean:
	rm -rf bin/
