.PHONY: build run mockllm mockllm-run clean

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

clean:
	rm -rf bin/
