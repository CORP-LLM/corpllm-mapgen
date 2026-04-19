package terrain

import (
	"math/rand"
	"testing"
	"time"
)

// randomConfig returns a test config with the given seed and sensible defaults.
// Rivers capped at 3, lakes capped at 5 — sufficient for coverage, fast to run.
func randomConfig(seed int64) *Config {
	return &Config{
		Seed:            seed,
		Width:           400,
		Height:          300,
		CellCount:       120,
		RelaxIterations: 2,
		Terrain: TerrainConfig{
			CoastEnabled:  true,
			CoastSide:     "south",
			CoastNoise:    0.5,
			WaterRatio:    0.3,
			RiversEnabled: true,
			RiverCount:    3,
			RiverWidth:    "medium",
			LakesEnabled:  true,
			LakeCount:     5,
			LakeSize:      "medium",
		},
	}
}

// TestRandomSeeds runs many random seeds and checks invariants on every result.
// Catches crashes and state-corruption bugs that a fixed seed would miss.
func TestRandomSeeds(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 20; i++ {
		seed := rng.Int63()
		cfg := randomConfig(seed)
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: Generate: %v", seed, err)
		}
		checkInvariants(t, tm, seed)
	}
}

// TestCoastSides verifies water clusters toward each configured side.
func TestCoastSides(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, side := range []string{"north", "south", "east", "west"} {
		for i := 0; i < 3; i++ {
			cfg := randomConfig(rng.Int63())
			cfg.Terrain.CoastSide = side
			cfg.Terrain.RiversEnabled = false
			cfg.Terrain.LakesEnabled = false
			tm, err := Generate(cfg)
			if err != nil {
				t.Fatalf("side=%s seed=%d: %v", side, cfg.Seed, err)
			}
			checkCoastDirection(t, tm, side, cfg)
		}
	}
}

// TestCoastNoInlandWater stresses the inland-cutoff fix: every coast water
// cell must be reachable from a map-border water cell via water neighbors.
func TestCoastNoInlandWater(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 15; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.RiversEnabled = false
		cfg.Terrain.LakesEnabled = false
		cfg.Terrain.CoastNoise = 0.7 // high noise stresses the cutoff
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		checkWaterConnectedToBorder(t, tm, cfg)
	}
}

// TestWaterRatioRange tests several water ratios across random seeds.
func TestWaterRatioRange(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, r := range []float64{0.15, 0.3, 0.5, 0.7} {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.WaterRatio = r
		cfg.Terrain.RiversEnabled = false
		cfg.Terrain.LakesEnabled = false
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("ratio=%.2f seed=%d: %v", r, cfg.Seed, err)
		}
		waterCount := 0
		for _, c := range tm.Cells {
			if c.Terrain == "water" {
				waterCount++
			}
		}
		got := float64(waterCount) / float64(len(tm.Cells))
		if diff := got - r; diff > 0.08 || diff < -0.08 {
			t.Errorf("ratio=%.2f seed=%d: got %.2f (tolerance ±0.08)", r, cfg.Seed, got)
		}
	}
}

// TestRiverCellsAreWater verifies every river cell has terrain="water".
// Rivers are generated as connected water cells, not as a separate land marker.
func TestRiverCellsAreWater(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.LakesEnabled = false
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, c := range tm.Cells {
			if c.River && c.Terrain != "water" {
				t.Errorf("seed %d: cell %d has river=true but terrain=%q (want water)",
					cfg.Seed, c.ID, c.Terrain)
			}
		}
	}
}

// TestParameterSweep varies cell count and relax iterations with random seeds.
func TestParameterSweep(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	cases := []struct {
		cellCount int
		relax     int
	}{
		{60, 0},
		{150, 3},
		{300, 5},
		{500, 1},
	}
	for _, tc := range cases {
		cfg := randomConfig(rng.Int63())
		cfg.CellCount = tc.cellCount
		cfg.RelaxIterations = tc.relax
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("cells=%d relax=%d seed=%d: %v", tc.cellCount, tc.relax, cfg.Seed, err)
		}
		if len(tm.Cells) != tc.cellCount {
			t.Errorf("cells=%d: got %d", tc.cellCount, len(tm.Cells))
		}
		checkInvariants(t, tm, cfg.Seed)
	}
}

// TestLakesIsolatedFromOtherWater verifies lakes never touch coast water,
// rivers, or other lakes — lake cells only border their own cluster or land.
func TestLakesIsolatedFromOtherWater(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 15; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Lakes) == 0 {
			continue
		}
		// Group lake cells by their lake ID.
		cellToLake := make(map[int]int, len(tm.Cells))
		for _, lk := range tm.Lakes {
			for _, cid := range lk.Cells {
				cellToLake[cid] = lk.ID
			}
		}
		// Build neighbor map.
		neighbors := make(map[int][]int, len(tm.Cells))
		for _, e := range tm.Edges {
			neighbors[e.Cells[0]] = append(neighbors[e.Cells[0]], e.Cells[1])
			neighbors[e.Cells[1]] = append(neighbors[e.Cells[1]], e.Cells[0])
		}
		// Every lake cell's water neighbors must be in the same lake.
		for _, lk := range tm.Lakes {
			for _, cid := range lk.Cells {
				for _, nb := range neighbors[cid] {
					if tm.Cells[nb].Terrain != "water" {
						continue
					}
					if cellToLake[nb] != lk.ID {
						t.Errorf("seed %d: lake %d cell %d has non-cluster water neighbor %d (lake=%d, river=%t)",
							cfg.Seed, lk.ID, cid, nb, cellToLake[nb], tm.Cells[nb].River)
					}
				}
			}
		}
	}
}

// TestLakeCellsMarked verifies the Cell.Lake flag is set for every cell
// listed in Terrain.Lakes — the frontend depends on this for rendering.
func TestLakeCellsMarked(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, lk := range tm.Lakes {
			for _, cid := range lk.Cells {
				c := tm.Cells[cid]
				if !c.Lake {
					t.Errorf("seed %d: lake %d cell %d has Lake=false", cfg.Seed, lk.ID, cid)
				}
				if c.Terrain != "water" {
					t.Errorf("seed %d: lake %d cell %d terrain=%q", cfg.Seed, lk.ID, cid, c.Terrain)
				}
			}
		}
		// No non-lake cell should have Lake=true.
		inLake := make(map[int]bool)
		for _, lk := range tm.Lakes {
			for _, cid := range lk.Cells {
				inLake[cid] = true
			}
		}
		for _, c := range tm.Cells {
			if c.Lake && !inLake[c.ID] {
				t.Errorf("seed %d: cell %d has Lake=true but not in any lake", cfg.Seed, c.ID)
			}
		}
	}
}

// TestLakeBordersNotCoastline verifies lake cell borders are not marked
// as coastline — only ocean shorelines should be coastline.
func TestLakeBordersNotCoastline(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, e := range tm.Edges {
			if !e.Coastline {
				continue
			}
			ca, cb := tm.Cells[e.Cells[0]], tm.Cells[e.Cells[1]]
			if ca.Lake || cb.Lake {
				t.Errorf("seed %d: edge %d is coastline but adjacent to lake cell", cfg.Seed, e.ID)
			}
			if ca.River || cb.River {
				t.Errorf("seed %d: edge %d is coastline but adjacent to river cell", cfg.Seed, e.ID)
			}
		}
	}
}

// TestCoastNoiseExtremes tests both ends of the coast-noise spectrum.
func TestCoastNoiseExtremes(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, noise := range []float64{0.0, 0.1, 0.9, 1.0} {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.CoastNoise = noise
		cfg.Terrain.RiversEnabled = false
		cfg.Terrain.LakesEnabled = false
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("noise=%.1f seed=%d: %v", noise, cfg.Seed, err)
		}
		checkInvariants(t, tm, cfg.Seed)
		// Even with max noise, inland-cutoff should keep water connected.
		checkWaterConnectedToBorder(t, tm, cfg)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func checkInvariants(t *testing.T, tm *Terrain, seed int64) {
	t.Helper()
	w, h := float64(tm.Bounds.Width), float64(tm.Bounds.Height)

	for _, c := range tm.Cells {
		if c.Terrain != "land" && c.Terrain != "water" {
			t.Errorf("seed %d: cell %d terrain=%q", seed, c.ID, c.Terrain)
		}
		if c.Center.X < 0 || c.Center.X > w || c.Center.Y < 0 || c.Center.Y > h {
			t.Errorf("seed %d: cell %d center (%.1f,%.1f) out of bounds",
				seed, c.ID, c.Center.X, c.Center.Y)
		}
		for _, v := range c.Vertices {
			if v.X < -1 || v.X > w+1 || v.Y < -1 || v.Y > h+1 {
				t.Errorf("seed %d: cell %d vertex (%.1f,%.1f) out of bounds",
					seed, c.ID, v.X, v.Y)
			}
		}
		if c.River && c.Terrain != "water" {
			t.Errorf("seed %d: cell %d river=true terrain=%q (want water)",
				seed, c.ID, c.Terrain)
		}
	}

	terrainOf := make(map[int]string, len(tm.Cells))
	for _, c := range tm.Cells {
		terrainOf[c.ID] = c.Terrain
	}
	for _, e := range tm.Edges {
		ta, tb := terrainOf[e.Cells[0]], terrainOf[e.Cells[1]]
		want := ta + "-" + tb
		if ta == "water" && tb == "land" {
			want = "land-water"
		}
		if want != e.Type {
			t.Errorf("seed %d: edge %d type=%q, cells are %s+%s",
				seed, e.ID, e.Type, ta, tb)
		}
	}

	// Lake cells must all be water.
	for _, lk := range tm.Lakes {
		for _, cid := range lk.Cells {
			if tm.Cells[cid].Terrain != "water" {
				t.Errorf("seed %d: lake %d cell %d not water", seed, lk.ID, cid)
			}
		}
	}
}

// checkCoastDirection verifies the water-cell centroid is biased toward the
// configured coast side. Water should dominate that half of the map.
func checkCoastDirection(t *testing.T, tm *Terrain, side string, cfg *Config) {
	t.Helper()
	var cx, cy float64
	var n int
	for _, c := range tm.Cells {
		if c.Terrain == "water" {
			cx += c.Center.X
			cy += c.Center.Y
			n++
		}
	}
	if n == 0 {
		t.Errorf("seed %d side=%s: no water cells", cfg.Seed, side)
		return
	}
	cx /= float64(n)
	cy /= float64(n)
	w, h := float64(cfg.Width), float64(cfg.Height)
	switch side {
	case "north":
		if cy > h*0.5 {
			t.Errorf("seed %d side=north: water centroid Y=%.1f, expected <%.1f",
				cfg.Seed, cy, h*0.5)
		}
	case "south":
		if cy < h*0.5 {
			t.Errorf("seed %d side=south: water centroid Y=%.1f, expected >%.1f",
				cfg.Seed, cy, h*0.5)
		}
	case "west":
		if cx > w*0.5 {
			t.Errorf("seed %d side=west: water centroid X=%.1f, expected <%.1f",
				cfg.Seed, cx, w*0.5)
		}
	case "east":
		if cx < w*0.5 {
			t.Errorf("seed %d side=east: water centroid X=%.1f, expected >%.1f",
				cfg.Seed, cx, w*0.5)
		}
	}
}

// checkWaterConnectedToBorder BFS-expands from border water cells and fails
// if any water cell is unreachable (i.e. an isolated inland blob).
func checkWaterConnectedToBorder(t *testing.T, tm *Terrain, cfg *Config) {
	t.Helper()
	w, h := float64(cfg.Width), float64(cfg.Height)
	const margin = 2.0

	neighbors := make(map[int][]int, len(tm.Cells))
	for _, e := range tm.Edges {
		neighbors[e.Cells[0]] = append(neighbors[e.Cells[0]], e.Cells[1])
		neighbors[e.Cells[1]] = append(neighbors[e.Cells[1]], e.Cells[0])
	}

	seen := make(map[int]bool, len(tm.Cells))
	var queue []int
	for _, c := range tm.Cells {
		if c.Terrain != "water" {
			continue
		}
		for _, v := range c.Vertices {
			if v.X <= margin || v.X >= w-margin ||
				v.Y <= margin || v.Y >= h-margin {
				seen[c.ID] = true
				queue = append(queue, c.ID)
				break
			}
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range neighbors[cur] {
			if seen[nb] || tm.Cells[nb].Terrain != "water" {
				continue
			}
			seen[nb] = true
			queue = append(queue, nb)
		}
	}

	for _, c := range tm.Cells {
		if c.Terrain == "water" && !seen[c.ID] {
			t.Errorf("seed %d: cell %d at (%.1f,%.1f) is inland water (not reachable from border)",
				cfg.Seed, c.ID, c.Center.X, c.Center.Y)
		}
	}
}
