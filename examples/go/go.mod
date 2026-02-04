module github.com/langdag/langdag/examples/go

go 1.21

require github.com/langdag/langdag-go v0.0.0

// Use the local SDK during development
replace github.com/langdag/langdag-go => ../../sdks/go
