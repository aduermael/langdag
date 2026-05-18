package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// CatalogV1SemanticJSON returns a stable JSON representation of catalog data
// with generation-time fields removed. It is used by publishing automation to
// avoid commits when only timestamps changed.
func CatalogV1SemanticJSON(data []byte) ([]byte, error) {
	catalog, err := ParseRemoteCatalogV1(data)
	if err != nil {
		return nil, err
	}
	NormalizeCatalogV1(catalog)
	normalized, err := json.Marshal(catalog)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(normalized, &value); err != nil {
		return nil, err
	}
	stripCatalogV1VolatileFields(value)
	return json.Marshal(value)
}

// CatalogV1SemanticEqual reports whether two catalog files differ in real
// model/deployment content, ignoring generation-time fields.
func CatalogV1SemanticEqual(leftPath, rightPath string) (bool, error) {
	left, err := os.ReadFile(leftPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", leftPath, err)
	}
	right, err := os.ReadFile(rightPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", rightPath, err)
	}
	leftSemantic, err := CatalogV1SemanticJSON(left)
	if err != nil {
		return false, fmt.Errorf("normalize %s: %w", leftPath, err)
	}
	rightSemantic, err := CatalogV1SemanticJSON(right)
	if err != nil {
		return false, fmt.Errorf("normalize %s: %w", rightPath, err)
	}
	return bytes.Equal(leftSemantic, rightSemantic), nil
}

func stripCatalogV1VolatileFields(value any) {
	switch v := value.(type) {
	case map[string]any:
		delete(v, "generated_at")
		delete(v, "stale_after")
		delete(v, "updated_at")
		delete(v, "observed_at")
		delete(v, "effective_at")
		for _, child := range v {
			stripCatalogV1VolatileFields(child)
		}
	case []any:
		for _, child := range v {
			stripCatalogV1VolatileFields(child)
		}
	}
}
