package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Response limits to prevent context explosion
const (
	// MaxListItems limits the number of items returned in list operations
	MaxListItems = 50
	// MaxEvents limits the number of events returned
	MaxEvents = 20
	// MaxDiffResources limits the number of resources in diff output
	MaxDiffResources = 20
	// MaxManifests limits the number of manifests returned
	MaxManifests = 20
	// MaxResponseLines limits the maximum lines in any response field
	MaxResponseLines = 100
	// MaxResponseSizeChars limits the maximum characters in any response string
	MaxResponseSizeChars = 50000
)

// Result returns a JSON-formatted result
func Result(data interface{}, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Truncate data to prevent context explosion
	data = truncateResponse(data)

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

	// Truncate items to prevent context explosion
	itemsList := items.([]interface{})
	itemsList = truncateResponse(itemsList).([]interface{})

	response := listResponse{
		Items: itemsList,
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

// truncateString truncates a string to a maximum number of characters
func truncateString(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	if maxChars <= 3 {
		return strings.Repeat(".", maxChars)
	}
	return s[:maxChars-3] + "..."
}

// truncateLines truncates a multi-line string to a maximum number of lines
func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n") + "\n... (truncated)"
}

// truncateResponse truncates a response value to prevent context explosion
func truncateResponse(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		truncated := truncateString(v, MaxResponseSizeChars)
		truncated = truncateLines(truncated, MaxResponseLines)
		return truncated
	case []interface{}:
		if len(v) > MaxListItems {
			return v[:MaxListItems]
		}
		return v
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = truncateResponse(val)
		}
		return result
	default:
		return v
	}
}
