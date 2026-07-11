package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// propertySchema is a JSON-schema fragment for a single tool argument.
type propertySchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Items       *propertySchema            `json:"items,omitempty"`
	Properties  map[string]*propertySchema `json:"properties,omitempty"`
}

// decodeArgs decodes the raw MCP arguments map into a typed request struct.
// Decoding is lenient like the previous manual accessors: unknown fields are
// ignored and missing fields keep their zero value; defaults are applied by
// the caller.
func decodeArgs[T any](arguments map[string]interface{}) (T, error) {
	var out T
	data, err := json.Marshal(arguments)
	if err != nil {
		return out, fmt.Errorf("failed to encode arguments: %w", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("invalid arguments: %w", err)
	}
	return out, nil
}

// schemaFor generates the MCP input schema for a request struct type.
// Field metadata comes from struct tags:
//
//	json:"snake_case_name"  argument name (required)
//	desc:"..."              human-readable description
//	required:"true"         adds the field to the schema's required list
//	enum:"a,b,c"            allowed values for string fields
func schemaFor[T any]() mcp.ToolInputSchema {
	var zero T
	t := reflect.TypeOf(zero)
	properties, required := structProperties(t)
	return mcp.ToolInputSchema{
		Type:       "object",
		Properties: toPropertyMap(properties),
		Required:   required,
	}
}

// structProperties walks a struct type and builds its schema properties.
func structProperties(t reflect.Type) (map[string]*propertySchema, []string) {
	properties := map[string]*propertySchema{}
	var required []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := jsonFieldName(field)
		if name == "" {
			continue
		}
		prop := propertyFor(field.Type)
		prop.Description = field.Tag.Get("desc")
		if enum := field.Tag.Get("enum"); enum != "" {
			prop.Enum = strings.Split(enum, ",")
		}
		properties[name] = prop
		if field.Tag.Get("required") == "true" {
			required = append(required, name)
		}
	}
	return properties, required
}

// propertyFor maps a Go type to its JSON-schema property.
func propertyFor(t reflect.Type) *propertySchema {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return &propertySchema{Type: "string"}
	case reflect.Bool:
		return &propertySchema{Type: "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64:
		return &propertySchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &propertySchema{Type: "number"}
	case reflect.Slice:
		return &propertySchema{Type: "array", Items: propertyFor(t.Elem())}
	case reflect.Struct:
		properties, _ := structProperties(t)
		return &propertySchema{Type: "object", Properties: properties}
	default:
		return &propertySchema{Type: "object"}
	}
}

// boolDefault dereferences an optional boolean argument, falling back to the
// given default when the argument was omitted. Used for booleans whose
// default is true (a plain bool cannot distinguish "absent" from "false").
func boolDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// jsonFieldName extracts the argument name from the json tag.
func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	if idx := strings.Index(tag, ","); idx >= 0 {
		tag = tag[:idx]
	}
	return tag
}

// toPropertyMap converts typed property schemas to the loosely-typed map the
// mcp-go library requires at its API boundary.
func toPropertyMap(properties map[string]*propertySchema) map[string]interface{} {
	data, err := json.Marshal(properties)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}
