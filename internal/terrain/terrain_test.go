package terrain

import (
	"testing"
)

func baseConfig() *Config {
	return &Config{
		Seed:            42,
		Width:           400,
		Height:          300,
		CellCount:       80,
		RelaxIterations: 1,
		Terrain: TerrainConfig{
			CoastEnabled:  true,
			CoastSide:     "south",
			CoastNoise:    0.5,
			WaterRatio:    0.25,
			RiversEnabled: true,
			Rivers: []RiverSpec{
				{Width: "medium", Origin: "border", End: "coast"},
				{Width: "medium", Origin: "border", End: "coast"},
			},
			LakesEnabled: true,
			Lakes: []LakeSpec{
				{Size: "medium"},
				{Size: "medium"},
			},
		},
	}
}

func TestGenerateBasic(t *testing.T) {
	cfg := baseConfig()
	tm, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(tm.Cells) != cfg.CellCount {
		t.Errorf("cells: want %d, got %d", cfg.CellCount, len(tm.Cells))
	}
	if len(tm.Edges) == 0 {
		t.Error("no edges generated")
	}
	if tm.Meta.ID == "" {
		t.Error("empty meta ID")
	}
}

func TestReproducibility(t *testing.T) {
	cfg := baseConfig()
	a, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(baseConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Cells) != len(b.Cells) {
		t.Fatalf("cell counts differ: %d vs %d", len(a.Cells), len(b.Cells))
	}
	for i := range a.Cells {
		ca, cb := a.Cells[i], b.Cells[i]
		if ca.Terrain != cb.Terrain {
			t.Errorf("cell %d terrain differs: %s vs %s", i, ca.Terrain, cb.Terrain)
		}
		if ca.Center.X != cb.Center.X || ca.Center.Y != cb.Center.Y {
			t.Errorf("cell %d center differs", i)
		}
	}
}

func TestCellsInsideBounds(t *testing.T) {
	cfg := baseConfig()
	tm, _ := Generate(cfg)
	w, h := float64(cfg.Width), float64(cfg.Height)
	for _, c := range tm.Cells {
		if c.Center.X < 0 || c.Center.X > w || c.Center.Y < 0 || c.Center.Y > h {
			t.Errorf("cell %d center outside bounds: (%.1f, %.1f)", c.ID, c.Center.X, c.Center.Y)
		}
		for _, v := range c.Vertices {
			if v.X < -1 || v.X > w+1 || v.Y < -1 || v.Y > h+1 {
				t.Errorf("cell %d vertex outside bounds: (%.1f, %.1f)", c.ID, v.X, v.Y)
			}
		}
	}
}

func TestNeighborsSymmetric(t *testing.T) {
	cfg := baseConfig()
	tm, _ := Generate(cfg)
	// Build reverse map.
	hasNeighbor := make(map[[2]int]bool, len(tm.Edges)*2)
	for _, e := range tm.Edges {
		hasNeighbor[[2]int{e.Cells[0], e.Cells[1]}] = true
		hasNeighbor[[2]int{e.Cells[1], e.Cells[0]}] = true
	}
	for _, c := range tm.Cells {
		for _, nb := range c.Neighbors {
			if !hasNeighbor[[2]int{c.ID, nb}] {
				t.Errorf("cell %d lists neighbor %d but no edge exists", c.ID, nb)
			}
		}
	}
}

func TestCoastWaterRatio(t *testing.T) {
	// Without lakes, water ratio should be close to requested.
	cfg := baseConfig()
	cfg.Terrain.LakesEnabled = false
	cfg.Terrain.RiversEnabled = false
	tm, _ := Generate(cfg)
	waterCount := 0
	for _, c := range tm.Cells {
		if c.Terrain == "water" {
			waterCount++
		}
	}
	ratio := float64(waterCount) / float64(len(tm.Cells))
	want := cfg.Terrain.WaterRatio
	if diff := ratio - want; diff > 0.05 || diff < -0.05 {
		t.Errorf("water ratio: want %.2f ±0.05, got %.2f", want, ratio)
	}
}

func TestRiversConnected(t *testing.T) {
	cfg := baseConfig()
	cfg.Terrain.LakesEnabled = false
	tm, _ := Generate(cfg)
	if len(tm.Rivers) == 0 {
		t.Skip("no rivers generated")
	}
	// Build edge adjacency.
	edgeByID := make(map[int]Edge, len(tm.Edges))
	for _, e := range tm.Edges {
		edgeByID[e.ID] = e
	}
	for _, rv := range tm.Rivers {
		if len(rv.Path) < 1 {
			t.Errorf("river %d has empty path", rv.ID)
			continue
		}
		// Consecutive edges must share a cell.
		for k := 0; k < len(rv.Path)-1; k++ {
			ea := edgeByID[rv.Path[k]]
			eb := edgeByID[rv.Path[k+1]]
			shared := false
			for _, ca := range ea.Cells {
				for _, cb := range eb.Cells {
					if ca == cb {
						shared = true
					}
				}
			}
			if !shared {
				t.Errorf("river %d: edges %d and %d not adjacent", rv.ID, ea.ID, eb.ID)
			}
		}
	}
}

func TestLakesAreClusters(t *testing.T) {
	cfg := baseConfig()
	cfg.Terrain.RiversEnabled = false
	tm, _ := Generate(cfg)
	if len(tm.Lakes) == 0 {
		t.Skip("no lakes generated")
	}
	// Build cell terrain map.
	terrainOf := make(map[int]string, len(tm.Cells))
	for _, c := range tm.Cells {
		terrainOf[c.ID] = c.Terrain
	}
	for _, lk := range tm.Lakes {
		for _, cid := range lk.Cells {
			if terrainOf[cid] != "water" {
				t.Errorf("lake %d: cell %d not water", lk.ID, cid)
			}
		}
		if lk.Area != len(lk.Cells) {
			t.Errorf("lake %d: area %d != len(cells) %d", lk.ID, lk.Area, len(lk.Cells))
		}
	}
}

func TestEdgeTypesConsistent(t *testing.T) {
	cfg := baseConfig()
	tm, _ := Generate(cfg)
	terrainOf := make(map[int]string, len(tm.Cells))
	for _, c := range tm.Cells {
		terrainOf[c.ID] = c.Terrain
	}
	for _, e := range tm.Edges {
		ta := terrainOf[e.Cells[0]]
		tb := terrainOf[e.Cells[1]]
		want := ta + "-" + tb
		if ta == "water" && tb == "land" {
			want = "land-water"
		}
		if want != e.Type {
			t.Errorf("edge %d: cells are %s+%s but type=%s", e.ID, ta, tb, e.Type)
		}
	}
}

func TestValidationRejectsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"cellCount too low", func(c *Config) { c.CellCount = 10 }},
		{"width too large", func(c *Config) { c.Width = 99999 }},
		{"bad coastSide", func(c *Config) { c.Terrain.CoastSide = "diagonal" }},
		{"waterRatio >1", func(c *Config) { c.Terrain.WaterRatio = 1.5 }},
		{"bad river width", func(c *Config) {
			c.Terrain.Rivers = []RiverSpec{{Width: "xxl"}}
		}},
		{"bad river origin", func(c *Config) {
			c.Terrain.Rivers = []RiverSpec{{Origin: "corner"}}
		}},
		{"bad lake size", func(c *Config) {
			c.Terrain.Lakes = []LakeSpec{{Size: "huge"}}
		}},
	}
	for _, tc := range cases {
		cfg := baseConfig()
		tc.mutate(cfg)
		if _, err := Generate(cfg); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}
