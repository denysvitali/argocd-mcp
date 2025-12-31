package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defaultv string
		expected string
	}{
		{
			name:     "string value exists",
			args:     map[string]interface{}{"key": "value"},
			key:      "key",
			defaultv: "default",
			expected: "value",
		},
		{
			name:     "string value missing",
			args:     map[string]interface{}{},
			key:      "key",
			defaultv: "default",
			expected: "default",
		},
		{
			name:     "string value wrong type",
			args:     map[string]interface{}{"key": 123},
			key:      "key",
			defaultv: "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.args, tt.key, tt.defaultv)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defaultv bool
		expected bool
	}{
		{
			name:     "bool value exists true",
			args:     map[string]interface{}{"key": true},
			key:      "key",
			defaultv: false,
			expected: true,
		},
		{
			name:     "bool value exists false",
			args:     map[string]interface{}{"key": false},
			key:      "key",
			defaultv: true,
			expected: false,
		},
		{
			name:     "bool value missing",
			args:     map[string]interface{}{},
			key:      "key",
			defaultv: true,
			expected: true,
		},
		{
			name:     "bool value wrong type",
			args:     map[string]interface{}{"key": "true"},
			key:      "key",
			defaultv: false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Bool(tt.args, tt.key, tt.defaultv)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defaultv int
		expected int
	}{
		{
			name:     "int value exists",
			args:     map[string]interface{}{"key": float64(42)},
			key:      "key",
			defaultv: 0,
			expected: 42,
		},
		{
			name:     "int value missing",
			args:     map[string]interface{}{},
			key:      "key",
			defaultv: 10,
			expected: 10,
		},
		{
			name:     "int value wrong type",
			args:     map[string]interface{}{"key": "not an int"},
			key:      "key",
			defaultv: 5,
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int(tt.args, tt.key, tt.defaultv)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFloat64(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		defaultv float64
		expected float64
	}{
		{
			name:     "float64 value exists",
			args:     map[string]interface{}{"key": 3.14},
			key:      "key",
			defaultv: 0.0,
			expected: 3.14,
		},
		{
			name:     "float64 value missing",
			args:     map[string]interface{}{},
			key:      "key",
			defaultv: 1.5,
			expected: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float64(tt.args, tt.key, tt.defaultv)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMap(t *testing.T) {
	t.Run("map value exists", func(t *testing.T) {
		args := map[string]interface{}{
			"config": map[string]interface{}{
				"key": "value",
			},
		}
		result := Map(args, "config")
		assert.NotNil(t, result)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("map value missing", func(t *testing.T) {
		args := map[string]interface{}{}
		result := Map(args, "config")
		assert.Nil(t, result)
	})
}

func TestMapSlice(t *testing.T) {
	t.Run("slice value exists", func(t *testing.T) {
		args := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "item1"},
				map[string]interface{}{"name": "item2"},
			},
		}
		result := MapSlice(args, "items")
		assert.NotNil(t, result)
		assert.Len(t, result, 2)
	})

	t.Run("slice value missing", func(t *testing.T) {
		args := map[string]interface{}{}
		result := MapSlice(args, "items")
		assert.Nil(t, result)
	})
}

func TestStringSlice(t *testing.T) {
	t.Run("string slice value exists", func(t *testing.T) {
		args := map[string]interface{}{
			"tags": []interface{}{"tag1", "tag2", "tag3"},
		}
		result := StringSlice(args, "tags")
		assert.NotNil(t, result)
		assert.Len(t, result, 3)
		assert.Equal(t, "tag1", result[0])
	})

	t.Run("string slice value missing", func(t *testing.T) {
		args := map[string]interface{}{}
		result := StringSlice(args, "tags")
		assert.Nil(t, result)
	})
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int64
		expected string
	}{
		{
			name:     "zero seconds",
			seconds:  0,
			expected: "N/A",
		},
		{
			name:     "valid seconds",
			seconds:  120,
			expected: "120 seconds ago",
		},
		{
			name:     "large value",
			seconds:  3600,
			expected: "3600 seconds ago",
		},
		{
			name:     "one hour",
			seconds:  3600,
			expected: "3600 seconds ago",
		},
		{
			name:     "one day",
			seconds:  86400,
			expected: "86400 seconds ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTime(tt.seconds)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSafeMode(t *testing.T) {
	// Create a tool manager with safe mode enabled
	tmSafe := &ToolManager{safeMode: true}
	tmUnsafe := &ToolManager{safeMode: false}

	t.Run("safe mode blocks operation", func(t *testing.T) {
		result := tmSafe.checkSafeMode("test_operation")
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("safe mode allows operation when disabled", func(t *testing.T) {
		result := tmUnsafe.checkSafeMode("test_operation")
		assert.Nil(t, result)
	})

	t.Run("safe mode is set correctly", func(t *testing.T) {
		assert.True(t, tmSafe.safeMode)
		assert.False(t, tmUnsafe.safeMode)
	})
}
