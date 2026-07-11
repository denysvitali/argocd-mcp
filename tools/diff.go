package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	yaml "sigs.k8s.io/yaml"
)

// stripManagedFieldsYaml removes managedFields from a YAML manifest
func stripManagedFieldsYaml(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonToYaml(jsonStr)
	}
	// Remove managedFields if present
	delete(data, "managedFields")
	// Re-marshal to JSON then to YAML
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return jsonToYaml(jsonStr)
	}
	return jsonToYaml(string(jsonBytes))
}

// computeDiff generates a human-readable diff between two YAML manifests
func computeDiff(target, live string) string {
	if target == "" || live == "" {
		return ""
	}
	// Parse both YAML documents
	var targetMap, liveMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(target), &targetMap); err != nil {
		return ""
	}
	if err := yaml.Unmarshal([]byte(live), &liveMap); err != nil {
		return ""
	}

	// Build diff by comparing values
	var diffLines []string
	compareMaps("", targetMap, liveMap, &diffLines)

	if len(diffLines) == 0 {
		return ""
	}
	return strings.Join(diffLines, "\n")
}

// compareMaps recursively compares two maps and adds differences to diffLines
func compareMaps(path string, target, live map[string]interface{}, diffLines *[]string) {
	// Check for removed or changed fields
	for key, tVal := range target {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}
		lVal, exists := live[key]
		if !exists {
			*diffLines = append(*diffLines, fmt.Sprintf("  %s: %v (REMOVED)", currentPath, tVal))
		} else {
			compareValues(currentPath, tVal, lVal, diffLines)
		}
	}
	// Check for added fields
	for key, lVal := range live {
		if _, exists := target[key]; !exists {
			currentPath := key
			if path != "" {
				currentPath = path + "." + key
			}
			*diffLines = append(*diffLines, fmt.Sprintf("  %s: %v (ADDED)", currentPath, lVal))
		}
	}
}

// compareValues compares two values and adds differences to diffLines
func compareValues(path string, target, live interface{}, diffLines *[]string) {
	tMap, tIsMap := target.(map[string]interface{})
	lMap, lIsMap := live.(map[string]interface{})
	tSlice, tIsSlice := target.([]interface{})
	lSlice, lIsSlice := live.([]interface{})

	if tIsMap && lIsMap {
		compareMaps(path, tMap, lMap, diffLines)
	} else if tIsSlice && lIsSlice {
		compareSlices(path, tSlice, lSlice, diffLines)
	} else if fmt.Sprintf("%v", target) != fmt.Sprintf("%v", live) {
		*diffLines = append(*diffLines, fmt.Sprintf("  %s: %v -> %v", path, live, target))
	}
}

// compareSlices compares two slices and adds differences to diffLines
func compareSlices(path string, target, live []interface{}, diffLines *[]string) {
	maxLen := len(target)
	if len(live) > maxLen {
		maxLen = len(live)
	}
	for i := 0; i < maxLen; i++ {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if i >= len(target) {
			*diffLines = append(*diffLines, fmt.Sprintf("  %s: %v (ADDED)", itemPath, live[i]))
		} else if i >= len(live) {
			*diffLines = append(*diffLines, fmt.Sprintf("  %s: %v (REMOVED)", itemPath, target[i]))
		} else {
			compareValues(itemPath, target[i], live[i], diffLines)
		}
	}
}

// jsonToYaml converts JSON string to YAML string
func jsonToYaml(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// If JSON parsing fails, return original string
		return jsonStr
	}
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return jsonStr
	}
	return string(yamlBytes)
}
