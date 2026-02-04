// Package workflow provides workflow validation functionality.
package workflow

import (
	"fmt"
	"strings"

	"github.com/langdag/langdag/pkg/types"
)

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains the result of workflow validation.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// Validate validates a workflow definition.
func Validate(workflow *types.Workflow) ValidationResult {
	result := ValidationResult{Valid: true}

	// Check required fields
	if workflow.Name == "" {
		result.addError("name", "name is required")
	}

	if len(workflow.Nodes) == 0 {
		result.addError("nodes", "at least one node is required")
	}

	// Validate nodes
	nodeIDs := make(map[string]bool)
	var hasInput, hasOutput bool

	for i, node := range workflow.Nodes {
		// Check node ID
		if node.ID == "" {
			result.addError(fmt.Sprintf("nodes[%d].id", i), "node id is required")
			continue
		}

		// Check for duplicate IDs
		if nodeIDs[node.ID] {
			result.addError(fmt.Sprintf("nodes[%d].id", i), fmt.Sprintf("duplicate node id: %s", node.ID))
		}
		nodeIDs[node.ID] = true

		// Check node type
		if !isValidNodeType(node.Type) {
			result.addError(fmt.Sprintf("nodes[%d].type", i), fmt.Sprintf("invalid node type: %s", node.Type))
		}

		// Track input/output nodes
		if node.Type == types.NodeTypeInput {
			if hasInput {
				result.addError(fmt.Sprintf("nodes[%d]", i), "multiple input nodes are not allowed")
			}
			hasInput = true
		}
		if node.Type == types.NodeTypeOutput {
			if hasOutput {
				result.addError(fmt.Sprintf("nodes[%d]", i), "multiple output nodes are not allowed")
			}
			hasOutput = true
		}

		// Validate node-type specific fields
		switch node.Type {
		case types.NodeTypeLLM:
			// LLM nodes should have either a prompt or system
			// (model can come from defaults)
		case types.NodeTypeBranch:
			if node.Condition == "" {
				result.addError(fmt.Sprintf("nodes[%d].condition", i), "branch nodes require a condition")
			}
		case types.NodeTypeTool:
			if node.Handler == "" {
				result.addError(fmt.Sprintf("nodes[%d].handler", i), "tool nodes require a handler")
			}
		}
	}

	// Validate edges
	for i, edge := range workflow.Edges {
		if edge.From == "" {
			result.addError(fmt.Sprintf("edges[%d].from", i), "edge 'from' is required")
		} else if !nodeIDs[edge.From] {
			result.addError(fmt.Sprintf("edges[%d].from", i), fmt.Sprintf("unknown node: %s", edge.From))
		}

		if edge.To == "" {
			result.addError(fmt.Sprintf("edges[%d].to", i), "edge 'to' is required")
		} else if !nodeIDs[edge.To] {
			result.addError(fmt.Sprintf("edges[%d].to", i), fmt.Sprintf("unknown node: %s", edge.To))
		}

		// Check for self-loops
		if edge.From == edge.To && edge.From != "" {
			result.addError(fmt.Sprintf("edges[%d]", i), "self-loops are not allowed")
		}
	}

	// Check for cycles
	if len(workflow.Nodes) > 0 {
		if _, err := TopologicalSort(workflow); err != nil {
			result.addError("edges", err.Error())
		}
	}

	// Validate tools referenced by nodes
	toolNames := make(map[string]bool)
	for _, tool := range workflow.Tools {
		toolNames[tool.Name] = true
	}

	for i, node := range workflow.Nodes {
		for _, toolName := range node.Tools {
			if !toolNames[toolName] {
				result.addError(fmt.Sprintf("nodes[%d].tools", i), fmt.Sprintf("unknown tool: %s", toolName))
			}
		}
	}

	return result
}

// ValidateFile validates a workflow file.
func ValidateFile(path string) ValidationResult {
	workflow, err := ParseFile(path)
	if err != nil {
		return ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Field: "file", Message: err.Error()},
			},
		}
	}

	return Validate(workflow)
}

// addError adds an error to the validation result.
func (r *ValidationResult) addError(field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// isValidNodeType checks if a node type is valid.
func isValidNodeType(t types.NodeType) bool {
	switch t {
	case types.NodeTypeLLM,
		types.NodeTypeTool,
		types.NodeTypeBranch,
		types.NodeTypeMerge,
		types.NodeTypeInput,
		types.NodeTypeOutput:
		return true
	default:
		return false
	}
}

// FormatErrors formats validation errors as a string.
func (r ValidationResult) FormatErrors() string {
	if r.Valid {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Validation errors:\n")
	for _, err := range r.Errors {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", err.Field, err.Message))
	}
	return sb.String()
}
