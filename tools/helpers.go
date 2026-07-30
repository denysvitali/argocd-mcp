package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
	yaml "sigs.k8s.io/yaml"
)

// Response limits to prevent context explosion
const (
	// MaxListItems is the default number of items returned in list operations
	MaxListItems = 50
	// MaxLimit is the highest limit a caller may ask for. It matches the
	// "max: 100" the tool schemas advertise; before this was enforced the
	// documented ceiling was decorative.
	MaxLimit = 100
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

// Result returns a YAML-formatted result
func Result(data interface{}, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return errorFromRPC(err), nil
	}

	// Truncate data to prevent context explosion
	data = truncateResponse(data)

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(yamlData),
			},
		},
	}, nil
}

// ResultList returns a YAML-formatted result for lists
func ResultList(items interface{}, total int, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return errorFromRPC(err), nil
	}

	type listResponse struct {
		Items []interface{} `json:"items"`
		Total int           `json:"total"`
	}

	// Truncate items to prevent context explosion
	itemsList, ok := items.([]interface{})
	if !ok {
		return errorResult("invalid items type: expected []interface{}"), nil
	}
	truncated := truncateResponse(itemsList)
	if truncatedList, ok := truncated.([]interface{}); ok {
		itemsList = truncatedList
	}

	response := listResponse{
		Items: itemsList,
		Total: total,
	}

	yamlData, err := yaml.Marshal(response)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to format response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(yamlData),
			},
		},
	}, nil
}

// TextResult returns a plain text result
func TextResult(text string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}, nil
}

// errorResult returns an error result
func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: message,
			},
		},
		IsError: true,
	}
}

// errorFromRPC turns an ArgoCD API error into something worth putting in
// front of a language model.
//
// The raw form is transport noise the caller can neither act on nor
// meaningfully relay:
//
//	rpc error: code = NotFound desc = appprojects.argoproj.io "x" not found
//
// Everything before "desc = " is stripped, and the gRPC code becomes a
// short prefix only when it tells the caller something the message does
// not: that they lack permission, or that the request timed out.
func errorFromRPC(err error) *mcp.CallToolResult {
	if err == nil {
		return errorResult("unknown error")
	}

	msg := err.Error()
	if i := strings.LastIndex(msg, "desc = "); i >= 0 {
		msg = msg[i+len("desc = "):]
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = err.Error()
	}

	switch {
	case strings.Contains(err.Error(), "code = PermissionDenied"),
		strings.Contains(err.Error(), "code = Unauthenticated"):
		return errorResult("permission denied: " + msg)
	case strings.Contains(err.Error(), "code = DeadlineExceeded"):
		return errorResult("timed out: " + msg)
	case strings.Contains(err.Error(), "code = Unavailable"):
		return errorResult("ArgoCD unreachable: " + msg)
	}
	return errorResult(msg)
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

// Limit reads a caller-supplied list limit and forces it into a usable
// range: at least 1, at most maxValue, defaultValue when absent or absurd.
//
// Every list handler slices its results with this value, so an unclamped
// negative limit panics with "slice bounds out of range" and takes the
// whole server down with it. The callers here are language models, which
// emit -1 for "no limit" often enough that this must never be trusted.
func Limit(arguments map[string]interface{}, key string, defaultValue, maxValue int) int {
	limit := Int(arguments, key, defaultValue)
	if limit < 1 {
		return defaultValue
	}
	if limit > maxValue {
		return maxValue
	}
	return limit
}

// Int64 returns the int64 value of the argument
func Int64(arguments map[string]interface{}, key string, defaultValue int64) int64 {
	if val, ok := arguments[key]; ok {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
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
		// Backstop only: handlers apply their own limits, which are
		// clamped to MaxLimit. Truncating here used to be silent, so a
		// response could claim "limited: false" while quietly dropping
		// items; if this ever fires, say so in the payload.
		if len(v) > MaxLimit {
			truncated := make([]interface{}, 0, MaxLimit+1)
			truncated = append(truncated, v[:MaxLimit]...)
			truncated = append(truncated, map[string]interface{}{
				"_truncated": fmt.Sprintf("%d further items omitted by the %d-item response cap", len(v)-MaxLimit, MaxLimit),
			})
			return truncated
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
