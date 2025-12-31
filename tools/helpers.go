package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Result returns a JSON-formatted result
func Result(data interface{}, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return errorResult(err.Error()), nil
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

// ResultList returns a JSON-formatted result for lists
func ResultList(items interface{}, total int, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return errorResult(err.Error()), nil
	}

	type listResponse struct {
		Items []interface{} `json:"items"`
		Total int           `json:"total"`
	}

	response := listResponse{
		Items: items.([]interface{}),
		Total: total,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

// errorResult returns an error result
func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: message,
			},
		},
		IsError: true,
	}
}

// Bool returns the bool value of the argument
func Bool(arguments map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := arguments[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// String returns the string value of the argument
func String(arguments map[string]interface{}, key string, defaultValue string) string {
	if val, ok := arguments[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return defaultValue
}

// Int returns the int value of the argument
func Int(arguments map[string]interface{}, key string, defaultValue int) int {
	if val, ok := arguments[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return defaultValue
}

// Float64 returns the float64 value of the argument
func Float64(arguments map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := arguments[key]; ok {
		if v, ok := val.(float64); ok {
			return v
		}
	}
	return defaultValue
}

// Map returns the map value of the argument
func Map(arguments map[string]interface{}, key string) map[string]interface{} {
	if val, ok := arguments[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// MapSlice returns the []interface{} value of the argument
func MapSlice(arguments map[string]interface{}, key string) []interface{} {
	if val, ok := arguments[key]; ok {
		if s, ok := val.([]interface{}); ok {
			return s
		}
	}
	return nil
}

// StringSlice returns the []string value of the argument
func StringSlice(arguments map[string]interface{}, key string) []string {
	if val, ok := arguments[key]; ok {
		if s, ok := val.([]interface{}); ok {
			result := make([]string, len(s))
			for i, v := range s {
				if str, ok := v.(string); ok {
					result[i] = str
				}
			}
			return result
		}
	}
	return nil
}

// IsContextCancelled checks if the context is cancelled
func IsContextCancelled(ctx context.Context, logger *logrus.Logger) bool {
	select {
	case <-ctx.Done():
		if ctx.Err() != nil {
			logger.Debugf("Context cancelled: %v", ctx.Err())
		}
		return true
	default:
		return false
	}
}

// ProtoToMap converts a protobuf message to a map
func ProtoToMap(msg interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal protobuf: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return result, nil
}

// ProtoToInterfaceList converts a protobuf slice to an interface slice
func ProtoToInterfaceList(items interface{}) ([]interface{}, error) {
	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal protobuf: %w", err)
	}
	var result []interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return result, nil
}

// EmptyToNil converts an empty protobuf message to nil
func EmptyToNil(msg interface{}, err error) error {
	if err != nil {
		return err
	}
	switch msg.(type) {
	case *emptypb.Empty:
		return nil
	default:
		return nil
	}
}

// FormatTime formats the timestamp for display
func FormatTime(seconds int64) string {
	if seconds == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d seconds ago", seconds)
}
