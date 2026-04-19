package terrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
