// Package workflow provides workflow parsing functionality.
package workflow

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/langdag/langdag/pkg/types"
	"gopkg.in/yaml.v3"
)

// YAMLNode represents a node in YAML format.
type YAMLNode struct {
	ID        string   `yaml:"id"`
	Type      string   `yaml:"type"`
	Model     string   `yaml:"model,omitempty"`
	System    string   `yaml:"system,omitempty"`
	Prompt    string   `yaml:"prompt,omitempty"`
	Tools     []string `yaml:"tools,omitempty"`
	Handler   string   `yaml:"handler,omitempty"`
	Condition string   `yaml:"condition,omitempty"`
}

// YAMLEdge represents an edge in YAML format.
type YAMLEdge struct {
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	Condition string `yaml:"condition,omitempty"`
	Transform string `yaml:"transform,omitempty"`
}

// YAMLDefaults represents default settings in YAML format.
type YAMLDefaults struct {
	Provider    string  `yaml:"provider,omitempty"`
	Model       string  `yaml:"model,omitempty"`
	MaxTokens   int     `yaml:"max_tokens,omitempty"`
	Temperature float64 `yaml:"temperature,omitempty"`
}

// YAMLTool represents a tool definition in YAML format.
type YAMLTool struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	InputSchema map[string]interface{} `yaml:"input_schema"`
}

// YAMLWorkflow represents a workflow in YAML format.
type YAMLWorkflow struct {
	Name        string       `yaml:"name"`
	Version     int          `yaml:"version,omitempty"`
	Description string       `yaml:"description,omitempty"`
	Defaults    YAMLDefaults `yaml:"defaults,omitempty"`
	Tools       []YAMLTool   `yaml:"tools,omitempty"`
	Nodes       []YAMLNode   `yaml:"nodes"`
	Edges       []YAMLEdge   `yaml:"edges"`
}

// ParseFile parses a workflow from a YAML file.
func ParseFile(path string) (*types.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return Parse(data)
}

// Parse parses a workflow from YAML bytes.
func Parse(data []byte) (*types.Workflow, error) {
	var yamlWorkflow YAMLWorkflow
	if err := yaml.Unmarshal(data, &yamlWorkflow); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return convertYAMLWorkflow(&yamlWorkflow)
}

// convertYAMLWorkflow converts a YAMLWorkflow to a types.Workflow.
func convertYAMLWorkflow(yamlWorkflow *YAMLWorkflow) (*types.Workflow, error) {
	workflow := &types.Workflow{
		Name:        yamlWorkflow.Name,
		Version:     yamlWorkflow.Version,
		Description: yamlWorkflow.Description,
		Defaults: types.WorkflowDefaults{
			Provider:    yamlWorkflow.Defaults.Provider,
			Model:       yamlWorkflow.Defaults.Model,
			MaxTokens:   yamlWorkflow.Defaults.MaxTokens,
			Temperature: yamlWorkflow.Defaults.Temperature,
		},
	}

	// Convert tools
	for _, tool := range yamlWorkflow.Tools {
		schemaJSON, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool schema for %s: %w", tool.Name, err)
		}

		workflow.Tools = append(workflow.Tools, types.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schemaJSON,
		})
	}

	// Convert nodes
	for _, node := range yamlWorkflow.Nodes {
		workflow.Nodes = append(workflow.Nodes, types.WorkflowNode{
			ID:        node.ID,
			Type:      types.NodeType(node.Type),
			Model:     node.Model,
			System:    node.System,
			Prompt:    node.Prompt,
			Tools:     node.Tools,
			Handler:   node.Handler,
			Condition: node.Condition,
		})
	}

	// Convert edges
	for _, edge := range yamlWorkflow.Edges {
		workflow.Edges = append(workflow.Edges, types.Edge{
			From:      edge.From,
			To:        edge.To,
			Condition: edge.Condition,
			Transform: edge.Transform,
		})
	}

	return workflow, nil
}

// ToYAML converts a types.Workflow to YAML bytes.
func ToYAML(workflow *types.Workflow) ([]byte, error) {
	yamlWorkflow := &YAMLWorkflow{
		Name:        workflow.Name,
		Version:     workflow.Version,
		Description: workflow.Description,
		Defaults: YAMLDefaults{
			Provider:    workflow.Defaults.Provider,
			Model:       workflow.Defaults.Model,
			MaxTokens:   workflow.Defaults.MaxTokens,
			Temperature: workflow.Defaults.Temperature,
		},
	}

	// Convert tools
	for _, tool := range workflow.Tools {
		var schema map[string]interface{}
		json.Unmarshal(tool.InputSchema, &schema)

		yamlWorkflow.Tools = append(yamlWorkflow.Tools, YAMLTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schema,
		})
	}

	// Convert nodes
	for _, node := range workflow.Nodes {
		yamlWorkflow.Nodes = append(yamlWorkflow.Nodes, YAMLNode{
			ID:        node.ID,
			Type:      string(node.Type),
			Model:     node.Model,
			System:    node.System,
			Prompt:    node.Prompt,
			Tools:     node.Tools,
			Handler:   node.Handler,
			Condition: node.Condition,
		})
	}

	// Convert edges
	for _, edge := range workflow.Edges {
		yamlWorkflow.Edges = append(yamlWorkflow.Edges, YAMLEdge{
			From:      edge.From,
			To:        edge.To,
			Condition: edge.Condition,
			Transform: edge.Transform,
		})
	}

	return yaml.Marshal(yamlWorkflow)
}
