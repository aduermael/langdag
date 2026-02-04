# Contributing to LangDAG

Thank you for your interest in contributing to LangDAG! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful and constructive. We're all here to build great software.

## Getting Started

### Prerequisites

- Go 1.23 or higher
- Git

### Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/langdag.git
cd langdag

# Build
go build -o langdag ./cmd/langdag

# Run tests
go test ./...
```

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### 2. Make Your Changes

- Follow the existing code style
- Add tests for new functionality
- Update documentation as needed

### 3. Test Your Changes

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/cli/...
```

### 4. Commit Your Changes

Use clear, descriptive commit messages:

```bash
git commit -m "feat: add PostgreSQL storage backend"
git commit -m "fix: handle empty conversation gracefully"
git commit -m "docs: update CLI reference for new flags"
```

Commit message prefixes:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation only
- `refactor:` Code refactoring
- `test:` Adding or updating tests
- `chore:` Maintenance tasks

### 5. Push and Create a Pull Request

```bash
git push origin feature/your-feature-name
```

Then create a Pull Request on GitHub.

## Code Style

### Go Code

- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Keep functions focused and small

```go
// Good: Clear, focused function
func (s *Storage) GetDAG(ctx context.Context, id string) (*DAG, error) {
    // ...
}

// Bad: Unclear, doing too much
func (s *Storage) Process(x interface{}) interface{} {
    // ...
}
```

### Project Structure

```
langdag/
├── cmd/langdag/        # CLI entry point
├── internal/           # Private packages
│   ├── cli/           # CLI commands
│   ├── config/        # Configuration
│   ├── conversation/  # Chat session management
│   ├── executor/      # Workflow execution
│   ├── provider/      # LLM provider implementations
│   ├── storage/       # Data persistence
│   └── workflow/      # Workflow parsing & validation
├── pkg/types/         # Public type definitions
├── docs/              # Documentation
└── examples/          # Example workflows
```

## Adding a New Provider

1. Create a new package in `internal/provider/`
2. Implement the `Provider` interface:

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}
```

3. Add configuration support in `internal/config/`
4. Register the provider in the CLI
5. Add tests and documentation

## Adding a New Storage Backend

1. Create a new package in `internal/storage/`
2. Implement the `Storage` interface
3. Add migration support
4. Add configuration options
5. Add tests and documentation

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation as needed
- Ensure all tests pass
- Respond to review feedback promptly

## Reporting Issues

When reporting bugs, please include:

1. LangDAG version (`langdag version`)
2. Operating system and version
3. Steps to reproduce
4. Expected vs actual behavior
5. Any relevant logs or error messages

## Feature Requests

For feature requests:

1. Check existing issues first
2. Describe the use case
3. Explain the expected behavior
4. Consider implementation complexity

## Questions?

- Open an issue for bugs or features
- Start a discussion for questions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
