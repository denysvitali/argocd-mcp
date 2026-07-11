package tools

import (
	"testing"

	"github.com/sirupsen/logrus"
)

// TestHandlerRegistryCoversAllTools ensures every defined tool has a handler
// and every handler corresponds to a defined tool.
func TestHandlerRegistryCoversAllTools(t *testing.T) {
	tm := NewToolManager(nil, logrus.New(), false, true)
	registry := tm.handlerRegistry()

	names := tm.GetToolNames()
	defined := make(map[string]bool, len(names))
	for _, name := range names {
		defined[name] = true
		if _, ok := registry[name]; !ok {
			t.Errorf("tool %q is defined but has no handler in the registry", name)
		}
	}

	for name := range registry {
		if !defined[name] {
			t.Errorf("handler registered for %q but no tool definition exists", name)
		}
	}
}
