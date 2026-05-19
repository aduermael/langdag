package api

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIPromptToolsContract(t *testing.T) {
	schemas := openAPISchemas(t)

	base := schemaMap(t, schemas, "PromptRequestBase")
	tools := propertyMap(t, base, "tools")
	items := mapValue(t, tools, "items")
	if got := stringValue(t, items, "$ref"); got != "#/components/schemas/ToolDefinition" {
		t.Fatalf("PromptRequestBase.tools.items.$ref = %q", got)
	}
	if !stringSliceContains(stringSliceValue(t, base, "required"), "message") {
		t.Fatalf("PromptRequestBase.required should include message")
	}
	if _, ok := properties(base)["system_prompt"]; ok {
		t.Fatalf("PromptRequestBase should not include system_prompt")
	}

	for _, name := range []string{"PromptRequest", "NodePromptRequest"} {
		schema := schemaMap(t, schemas, name)
		allOf := sliceValue(t, schema, "allOf")
		if len(allOf) == 0 {
			t.Fatalf("%s should use allOf to share PromptRequestBase", name)
		}
		ref := stringValue(t, allOf[0].(map[string]any), "$ref")
		if ref != "#/components/schemas/PromptRequestBase" {
			t.Fatalf("%s allOf[0].$ref = %q", name, ref)
		}
	}
	_ = propertyMap(t, schemaMap(t, schemas, "PromptRequest"), "system_prompt")

	tool := schemaMap(t, schemas, "ToolDefinition")
	for _, property := range []string{"name", "description", "input_schema"} {
		_ = propertyMap(t, tool, property)
	}
	required := stringSliceValue(t, tool, "required")
	if len(required) != 1 || required[0] != "name" {
		t.Fatalf("ToolDefinition.required = %+v, want only name", required)
	}
}

func TestOpenAPIOutputGroupIDContract(t *testing.T) {
	schemas := openAPISchemas(t)
	for _, schemaName := range []string{"Node", "PromptResponse"} {
		outputGroupID := propertyMap(t, schemaMap(t, schemas, schemaName), "output_group_id")
		if got := stringValue(t, outputGroupID, "type"); got != "string" {
			t.Fatalf("%s.output_group_id.type = %q", schemaName, got)
		}
		if got := stringValue(t, outputGroupID, "format"); got != "uuid" {
			t.Fatalf("%s.output_group_id.format = %q", schemaName, got)
		}
	}
}

func openAPISchemas(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}
	return mapValue(t, mapValue(t, doc, "components"), "schemas")
}

func schemaMap(t *testing.T, schemas map[string]any, name string) map[string]any {
	t.Helper()
	return mapValue(t, schemas, name)
}

func propertyMap(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	return mapValue(t, properties(schema), name)
}

func properties(schema map[string]any) map[string]any {
	if direct, ok := schema["properties"].(map[string]any); ok {
		return direct
	}
	allOf, ok := schema["allOf"].([]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for _, part := range allOf {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		partProperties, ok := partMap["properties"].(map[string]any)
		if !ok {
			continue
		}
		for name, property := range partProperties {
			out[name] = property
		}
	}
	return out
}

func mapValue(t *testing.T, values map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := values[key]
	if !ok {
		t.Fatalf("missing map key %q", key)
	}
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("map key %q has type %T, want map[string]any", key, value)
	}
	return typed
}

func sliceValue(t *testing.T, values map[string]any, key string) []any {
	t.Helper()
	value, ok := values[key]
	if !ok {
		t.Fatalf("missing slice key %q", key)
	}
	typed, ok := value.([]any)
	if !ok {
		t.Fatalf("slice key %q has type %T, want []any", key, value)
	}
	return typed
}

func stringSliceValue(t *testing.T, values map[string]any, key string) []string {
	t.Helper()
	raw := sliceValue(t, values, key)
	out := make([]string, len(raw))
	for i, value := range raw {
		typed, ok := value.(string)
		if !ok {
			t.Fatalf("%s[%d] has type %T, want string", key, i, value)
		}
		out[i] = typed
	}
	return out
}

func stringValue(t *testing.T, values map[string]any, key string) string {
	t.Helper()
	value, ok := values[key]
	if !ok {
		t.Fatalf("missing string key %q", key)
	}
	typed, ok := value.(string)
	if !ok {
		t.Fatalf("string key %q has type %T, want string", key, value)
	}
	return typed
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
