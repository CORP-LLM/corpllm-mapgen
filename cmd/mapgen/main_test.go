package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateCommandEmptyStdin(t *testing.T) {
	// Empty input → uses all defaults.
	var out bytes.Buffer
	if err := runGenerate(strings.NewReader(""), &out); err != nil {
		t.Fatalf("generate empty: %v", err)
	}
	var tm struct {
		Meta struct {
			SchemaVersion string `json:"schemaVersion"`
		} `json:"meta"`
		Cells []struct{ ID int } `json:"cells"`
	}
	if err := json.Unmarshal(out.Bytes(), &tm); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if tm.Meta.SchemaVersion == "" {
		t.Error("output missing meta.schemaVersion")
	}
	if len(tm.Cells) == 0 {
		t.Error("output has zero cells")
	}
}

func TestGenerateCommandWithConfig(t *testing.T) {
	cfg := `{
		"seed": 42, "width": 400, "height": 300, "cellCount": 120,
		"relaxIterations": 2,
		"terrain": {
			"coastEnabled": true, "coastSide": "south", "coastNoise": 0.5,
			"waterRatio": 0.25, "roughness": 1.0,
			"riversEnabled": false, "rivers": null,
			"lakesEnabled": false, "lakes": null,
			"highwaysEnabled": false, "highways": null
		}
	}`
	var out bytes.Buffer
	if err := runGenerate(strings.NewReader(cfg), &out); err != nil {
		t.Fatalf("generate: %v", err)
	}
	var tm struct {
		Meta struct {
			Seed      int64 `json:"seed"`
			CellCount int   `json:"cellCount"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out.Bytes(), &tm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tm.Meta.Seed != 42 || tm.Meta.CellCount != 120 {
		t.Errorf("meta mismatch: seed=%d cellCount=%d", tm.Meta.Seed, tm.Meta.CellCount)
	}
}

func TestGenerateCommandBadJSON(t *testing.T) {
	var out bytes.Buffer
	err := runGenerate(strings.NewReader("{not json"), &out)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("error should mention parse config: %v", err)
	}
}
