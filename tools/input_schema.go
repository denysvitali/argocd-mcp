package tools

import "encoding/json"

// ToolInputSchema is the JSON Schema describing a tool's parameters.
//
// The official SDK types mcp.Tool.InputSchema as `any` (anything that
// marshals to a valid JSON Schema), so we keep our own literal-friendly
// struct rather than building github.com/google/jsonschema-go values by
// hand for ~45 tool definitions.
type ToolInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// MarshalJSON emits an empty object for Properties rather than null.
// Clients that validate the schema reject `"properties": null`, and tools
// that take no arguments would otherwise produce exactly that.
func (s ToolInputSchema) MarshalJSON() ([]byte, error) {
	type alias ToolInputSchema
	out := alias(s)
	if out.Type == "" {
		out.Type = "object"
	}
	if out.Properties == nil {
		out.Properties = map[string]interface{}{}
	}
	return json.Marshal(out)
}
