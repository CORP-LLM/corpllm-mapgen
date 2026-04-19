package terrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestSchemaParses ensures the published JSON Schema at schema/terrain.schema.json
// is valid JSON. The client team reads this schema to validate responses, so it
// must always round-trip cleanly.
func TestSchemaParses(t *testing.T) {
	// Test runs from internal/terrain/; schema lives at repo root.
	path := filepath.Join("..", "..", "schema", "terrain.schema.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read schema: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if schema["$id"] != "https://corpllm.io/schemas/terrain.schema.json" {
		t.Errorf("schema $id mismatch: %v", schema["$id"])
	}
	defs, ok := schema["$defs"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing $defs block")
	}
	// Key types the client relies on must all be defined.
	for _, must := range []string{
		"Cell", "Edge", "River", "Lake", "Highway", "Biome",
		"Meta", "Config", "TerrainConfig", "Bounds",
	} {
		if _, ok := defs[must]; !ok {
			t.Errorf("$defs missing required type %q", must)
		}
	}
}

// TestGeneratedTerrainValidatesAgainstSchema catches drift between the Go
// types and the published JSON Schema — every generated terrain must
// validate cleanly against schema/terrain.schema.json.
//
// Without this, a field added to a Go struct (or a changed enum value)
// could silently ship to clients that still validate the old schema.
func TestGeneratedTerrainValidatesAgainstSchema(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "schema", "terrain.schema.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("terrain.schema.json", doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	sch, err := compiler.Compile("terrain.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	// Generate with multiple config variants so every optional branch of
	// the schema is hit: no coast, no lakes, no rivers, highways, etc.
	variants := []func(*Config){
		func(c *Config) {}, // default
		func(c *Config) { c.Terrain.CoastEnabled = false },
		func(c *Config) { c.Terrain.LakesEnabled = false; c.Terrain.Lakes = nil },
		func(c *Config) { c.Terrain.RiversEnabled = false; c.Terrain.Rivers = nil },
		func(c *Config) {
			c.Terrain.HighwaysEnabled = true
			c.Terrain.Highways = []HighwaySpec{{From: "north", To: "south"}}
		},
		func(c *Config) { c.Terrain.Roughness = 0.2 }, // flat terrain
	}
	for i, mutate := range variants {
		cfg := baseConfig()
		mutate(cfg)
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("variant %d: generate: %v", i, err)
		}
		b, err := json.Marshal(tm)
		if err != nil {
			t.Fatalf("variant %d: marshal: %v", i, err)
		}
		var instance interface{}
		if err := json.Unmarshal(b, &instance); err != nil {
			t.Fatalf("variant %d: unmarshal for validation: %v", i, err)
		}
		if err := sch.Validate(instance); err != nil {
			t.Errorf("variant %d: schema validation failed:\n%v", i, err)
		}
	}
}

// TestGeneratedTerrainRoundTrips the output JSON must be serializable and
// deserializable cleanly — the schema's semantic types all survive.
func TestGeneratedTerrainRoundTrips(t *testing.T) {
	cfg := baseConfig()
	tm, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(tm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Terrain
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Meta.ID != tm.Meta.ID {
		t.Errorf("meta.id lost: %q vs %q", round.Meta.ID, tm.Meta.ID)
	}
	if len(round.Cells) != len(tm.Cells) {
		t.Errorf("cells count lost: %d vs %d", len(round.Cells), len(tm.Cells))
	}
	// Biome must be present and non-empty for every cell.
	for i, c := range round.Cells {
		if c.Biome == "" {
			t.Errorf("cell %d biome empty after round-trip", i)
			break
		}
	}
}
