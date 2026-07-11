package tools

import (
	"reflect"
	"testing"
)

type testArgs struct {
	Name       string            `json:"name" desc:"Application name (required)" required:"true"`
	Limit      int               `json:"limit" desc:"Maximum items"`
	Prune      bool              `json:"prune" desc:"Prune resources"`
	Cost       float64           `json:"cost"`
	Repos      []string          `json:"repos" desc:"Allowed repositories"`
	Refresh    string            `json:"refresh" enum:"normal,hard"`
	Dest       testDestination   `json:"dest"`
	DestList   []testDestination `json:"dest_list"`
	unexported string            //nolint:unused // verifies unexported fields are skipped
}

type testDestination struct {
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
}

func TestSchemaFor(t *testing.T) {
	schema := schemaFor[testArgs]()

	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if !reflect.DeepEqual(schema.Required, []string{"name"}) {
		t.Errorf("expected required=[name], got %v", schema.Required)
	}

	name, ok := schema.Properties["name"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing name property: %v", schema.Properties)
	}
	if name["type"] != "string" || name["description"] != "Application name (required)" {
		t.Errorf("unexpected name property: %v", name)
	}

	limit := schema.Properties["limit"].(map[string]interface{})
	if limit["type"] != "integer" {
		t.Errorf("expected integer limit, got %v", limit)
	}
	prune := schema.Properties["prune"].(map[string]interface{})
	if prune["type"] != "boolean" {
		t.Errorf("expected boolean prune, got %v", prune)
	}
	cost := schema.Properties["cost"].(map[string]interface{})
	if cost["type"] != "number" {
		t.Errorf("expected number cost, got %v", cost)
	}

	repos := schema.Properties["repos"].(map[string]interface{})
	if repos["type"] != "array" {
		t.Errorf("expected array repos, got %v", repos)
	}
	items := repos["items"].(map[string]interface{})
	if items["type"] != "string" {
		t.Errorf("expected string items, got %v", items)
	}

	refresh := schema.Properties["refresh"].(map[string]interface{})
	enum, ok := refresh["enum"].([]interface{})
	if !ok || len(enum) != 2 || enum[0] != "normal" || enum[1] != "hard" {
		t.Errorf("unexpected enum: %v", refresh["enum"])
	}

	dest := schema.Properties["dest"].(map[string]interface{})
	if dest["type"] != "object" {
		t.Errorf("expected object dest, got %v", dest)
	}
	destProps := dest["properties"].(map[string]interface{})
	if _, ok := destProps["server"]; !ok {
		t.Errorf("expected nested server property, got %v", destProps)
	}

	destList := schema.Properties["dest_list"].(map[string]interface{})
	listItems := destList["items"].(map[string]interface{})
	if listItems["type"] != "object" {
		t.Errorf("expected object list items, got %v", listItems)
	}

	if _, ok := schema.Properties["unexported"]; ok {
		t.Error("unexported field leaked into schema")
	}
}

func TestDecodeArgs(t *testing.T) {
	args, err := decodeArgs[testArgs](map[string]interface{}{
		"name":  "my-app",
		"limit": float64(25), // JSON numbers arrive as float64
		"prune": true,
		"repos": []interface{}{"a", "b"},
		"dest":  map[string]interface{}{"server": "https://k8s", "namespace": "default"},
		"extra": "ignored",
	})
	if err != nil {
		t.Fatalf("decodeArgs failed: %v", err)
	}
	if args.Name != "my-app" || args.Limit != 25 || !args.Prune {
		t.Errorf("unexpected decode: %+v", args)
	}
	if len(args.Repos) != 2 || args.Repos[0] != "a" {
		t.Errorf("unexpected repos: %v", args.Repos)
	}
	if args.Dest.Server != "https://k8s" {
		t.Errorf("unexpected dest: %+v", args.Dest)
	}
}

func TestDecodeArgsTypeMismatch(t *testing.T) {
	_, err := decodeArgs[testArgs](map[string]interface{}{"limit": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}
