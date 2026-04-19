package terrain

import (
	"math"
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
			Rivers: []RiverSpec{
				{Width: "medium", Origin: "border", End: "coast"},
				{Width: "medium", Origin: "border", End: "coast"},
				{Width: "medium", Origin: "border", End: "coast"},
			},
			LakesEnabled: true,
			Lakes: []LakeSpec{
				{Size: "medium"}, {Size: "medium"}, {Size: "medium"},
				{Size: "medium"}, {Size: "medium"},
			},
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
		// Lake cells may border river cells (rivers flow into lakes) but
		// must not border coast water or a DIFFERENT lake.
		for _, lk := range tm.Lakes {
			for _, cid := range lk.Cells {
				for _, nb := range neighbors[cid] {
					nc := tm.Cells[nb]
					if nc.Terrain != "water" || nc.River {
						continue
					}
					if cellToLake[nb] != lk.ID {
						kind := "coast"
						if nc.Lake {
							kind = "other lake"
						}
						t.Errorf("seed %d: lake %d cell %d has %s water neighbor %d",
							cfg.Seed, lk.ID, cid, kind, nb)
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

// TestRiverOriginInland verifies rivers with Origin="inland" start away from
// the map border (in the central region).
func TestRiverOriginInland(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.LakesEnabled = false
		cfg.Terrain.Rivers = []RiverSpec{
			{Width: "wide", Origin: "inland", End: "coast"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Rivers) == 0 {
			continue
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		inlandMargin := math.Min(w, h) * 0.3
		src := tm.Rivers[0].Source
		inX := src.X >= inlandMargin && src.X <= w-inlandMargin
		inY := src.Y >= inlandMargin && src.Y <= h-inlandMargin
		if !(inX && inY) {
			t.Errorf("seed %d: inland river source (%.1f,%.1f) not inland (margin %.1f)",
				cfg.Seed, src.X, src.Y, inlandMargin)
		}
	}
}

// TestRiverEndOffmap verifies end=offmap rivers end at a map border — but
// only when configured with border origin (source near a border, downhill
// routing can reach the opposite low-elev border). Inland→offmap isn't
// physically sensible under strict hydrology: mountain rivers flow toward
// the coast, not arbitrary borders.
func TestRiverEndOffmap(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var hits, tries int
	for i := 0; i < 20; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.LakesEnabled = false
		cfg.Terrain.CoastEnabled = false // no coast — river must exit via border
		cfg.Terrain.Rivers = []RiverSpec{
			{Width: "narrow", Origin: "border", End: "offmap", Straightness: 0.8},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Rivers) == 0 {
			continue
		}
		tries++
		mouth := tm.Rivers[0].Mouth
		for j := range tm.Cells {
			if tm.Cells[j].Center != mouth {
				continue
			}
			w, h := float64(cfg.Width), float64(cfg.Height)
			const margin = 2.0
			for _, v := range tm.Cells[j].Vertices {
				if v.X <= margin || v.X >= w-margin || v.Y <= margin || v.Y >= h-margin {
					hits++
					break
				}
			}
			break
		}
	}
	if tries == 0 {
		t.Skip("no rivers generated")
	}
	rate := float64(hits) / float64(tries)
	if rate < 0.5 {
		t.Errorf("offmap hit rate only %.0f%% (%d/%d)", rate*100, hits, tries)
	}
}

// TestRiverEndLake verifies that most rivers with End="lake" reach a lake.
// Greedy routing can get blocked by topology; we assert an aggregate success
// rate rather than per-seed termination.
func TestRiverEndLake(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var hits, tries int
	for i := 0; i < 20; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.CellCount = 250
		// Disable coast so the only drainage option is the lakes.
		cfg.Terrain.CoastEnabled = false
		cfg.Terrain.Lakes = []LakeSpec{{Size: "large"}, {Size: "large"}, {Size: "large"}}
		cfg.Terrain.Rivers = []RiverSpec{
			{Width: "medium", Origin: "inland", End: "lake"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Rivers) == 0 || len(tm.Lakes) == 0 {
			continue
		}
		tries++
		mouth := tm.Rivers[0].Mouth
		for j := range tm.Cells {
			if tm.Cells[j].Center == mouth && tm.Cells[j].Lake {
				hits++
				break
			}
		}
	}
	if tries == 0 {
		t.Skip("no rivers generated")
	}
	rate := float64(hits) / float64(tries)
	if rate < 0.5 {
		t.Errorf("end=lake hit rate only %.0f%% (%d/%d) — routing not preferring lakes",
			rate*100, hits, tries)
	}
}

// TestRiverStraightness verifies that high-straightness rivers use on average
// fewer cells than fully natural ones (a straighter path covers less ground).
// Aggregated over multiple seeds to smooth out topology variance.
func TestRiverStraightness(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var totalNatural, totalStraight int
	var samples int
	for i := 0; i < 12; i++ {
		seed := rng.Int63()
		cfgA := randomConfig(seed)
		cfgA.Terrain.LakesEnabled = false
		cfgA.Terrain.Rivers = []RiverSpec{
			{Width: "medium", Origin: "border", End: "coast", Straightness: 0.0},
		}
		tmA, _ := Generate(cfgA)

		cfgB := randomConfig(seed)
		cfgB.Terrain.LakesEnabled = false
		cfgB.Terrain.Rivers = []RiverSpec{
			{Width: "medium", Origin: "border", End: "coast", Straightness: 1.0},
		}
		tmB, _ := Generate(cfgB)

		if len(tmA.Rivers) == 0 || len(tmB.Rivers) == 0 {
			continue
		}
		cellsA, cellsB := 0, 0
		for _, c := range tmA.Cells {
			if c.River {
				cellsA++
			}
		}
		for _, c := range tmB.Cells {
			if c.River {
				cellsB++
			}
		}
		totalNatural += cellsA
		totalStraight += cellsB
		samples++
	}
	if samples == 0 {
		t.Skip("no rivers generated")
	}
	// Average: straight should be ≤ natural. Allow small slack (≤ 15%) for topology variance.
	avgN := float64(totalNatural) / float64(samples)
	avgS := float64(totalStraight) / float64(samples)
	if avgS > avgN*1.15 {
		t.Errorf("straight avg cells %.1f > natural avg cells %.1f + 15%%", avgS, avgN)
	}
}

// TestLakeSizesHonored verifies per-lake size configurations result in
// clusters of the expected approximate size.
func TestLakeSizesHonored(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	cfg := randomConfig(rng.Int63())
	cfg.Terrain.RiversEnabled = false
	cfg.CellCount = 300
	cfg.Terrain.Lakes = []LakeSpec{
		{Size: "large"}, {Size: "small"}, {Size: "small"},
	}
	tm, err := Generate(cfg)
	if err != nil {
		t.Fatalf("seed %d: %v", cfg.Seed, err)
	}
	if len(tm.Lakes) < 2 {
		t.Skipf("not enough lakes generated (got %d)", len(tm.Lakes))
	}
	// Large lake should be bigger than small lakes.
	expectedBiggest := tm.Lakes[0].Area
	for _, lk := range tm.Lakes[1:] {
		if lk.Area > expectedBiggest {
			t.Errorf("seed %d: small lake %d (size %d) is larger than large lake 0 (size %d)",
				cfg.Seed, lk.ID, lk.Area, expectedBiggest)
		}
	}
}

// TestCellVerticesInsideBounds guarantees all cell vertices stay inside the
// map rectangle — front-end rendering relies on this to keep the map border
// crisp even with subdivision wiggle.
func TestCellVerticesInsideBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		for _, c := range tm.Cells {
			for _, v := range c.Vertices {
				if v.X < -1 || v.X > w+1 || v.Y < -1 || v.Y > h+1 {
					t.Errorf("seed %d: cell %d vertex (%.2f,%.2f) outside map bounds",
						cfg.Seed, c.ID, v.X, v.Y)
				}
			}
		}
	}
}

// TestRiverMouthTouchesWater guarantees every river's mouth cell is water
// (coast or lake) OR the cell touches the map border (end=offmap). Rivers
// must have a hydrological terminus — they never dead-end on dry land.
func TestRiverMouthTouchesWater(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 15; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		cellByID := make(map[int]*Cell, len(tm.Cells))
		for j := range tm.Cells {
			cellByID[tm.Cells[j].ID] = &tm.Cells[j]
		}
		for ri, rv := range tm.Rivers {
			if len(rv.Path) == 0 {
				t.Errorf("seed %d: river %d has empty path", cfg.Seed, ri)
				continue
			}
			// Find the mouth cell via the last edge.
			last := tm.Edges[rv.Path[len(rv.Path)-1]]
			a, b := cellByID[last.Cells[0]], cellByID[last.Cells[1]]
			// Mouth is the non-river water OR border cell at the end.
			endSpec := cfg.Terrain.Rivers[rv.ID].End
			good := false
			for _, c := range []*Cell{a, b} {
				if c == nil {
					continue
				}
				if c.Terrain == "water" {
					good = true
				}
				if endSpec == "offmap" {
					for _, v := range c.Vertices {
						if v.X <= 2 || v.X >= w-2 || v.Y <= 2 || v.Y >= h-2 {
							good = true
						}
					}
				}
			}
			if !good {
				t.Errorf("seed %d: river %d mouth does not touch water or map border", cfg.Seed, ri)
			}
		}
	}
}

// TestRiverCellPathMatchesEdges verifies the CellPath in the API response
// is consistent with the edge Path — each consecutive cell pair shares the
// edge at the corresponding position.
func TestRiverCellPathMatchesEdges(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, rv := range tm.Rivers {
			if len(rv.CellPath) != len(rv.Path)+1 {
				t.Errorf("seed %d: river %d CellPath len %d, Path len %d (expected CellPath = Path+1)",
					cfg.Seed, rv.ID, len(rv.CellPath), len(rv.Path))
				continue
			}
			for k, eid := range rv.Path {
				e := tm.Edges[eid]
				a, b := rv.CellPath[k], rv.CellPath[k+1]
				match := (e.Cells[0] == a && e.Cells[1] == b) || (e.Cells[0] == b && e.Cells[1] == a)
				if !match {
					t.Errorf("seed %d: river %d edge[%d] cells %v don't connect %d→%d",
						cfg.Seed, rv.ID, k, e.Cells, a, b)
				}
			}
		}
	}
}

// TestRiverStylesProduceDifferentPaths verifies that the style-preset
// parameters actually change the generated river path via sinuosity
// (path length ÷ straight-line distance from source to mouth). Straighter
// styles should have sinuosity closer to 1.0; meandering ones higher.
func TestRiverStylesProduceDifferentPaths(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	sinuosity := func(rv *River, cells []Cell) float64 {
		pathLen := 0.0
		for i := 0; i < len(rv.CellPath)-1; i++ {
			a := cells[rv.CellPath[i]].Center
			b := cells[rv.CellPath[i+1]].Center
			pathLen += math.Hypot(b.X-a.X, b.Y-a.Y)
		}
		straight := math.Hypot(rv.Mouth.X-rv.Source.X, rv.Mouth.Y-rv.Source.Y)
		if straight < 5 {
			return 1
		}
		return pathLen / straight
	}
	runStyle := func(straight, meander float64) float64 {
		var sum float64
		count := 0
		for i := 0; i < 30; i++ {
			cfg := randomConfig(rng.Int63())
			cfg.CellCount = 300 // larger map → more meaningful paths
			cfg.Terrain.LakesEnabled = false
			cfg.Terrain.Rivers = []RiverSpec{
				{Width: "medium", Origin: "border", End: "coast",
					Straightness: straight, Meander: meander},
			}
			tm, _ := Generate(cfg)
			if len(tm.Rivers) > 0 && len(tm.Rivers[0].CellPath) >= 5 {
				sum += sinuosity(&tm.Rivers[0], tm.Cells)
				count++
			}
		}
		if count == 0 {
			return 0
		}
		return sum / float64(count)
	}
	straight := runStyle(1.0, 0)
	natural := runStyle(0, 0)
	if straight == 0 || natural == 0 {
		t.Skip("not enough rivers generated")
	}
	// Straightness=1 should produce measurably less winding paths than
	// straightness=0 (pure greedy downhill). Averaged over 30 seeds the
	// difference is small but consistent; keep threshold tight but not zero.
	if straight > natural {
		t.Errorf("straight sinuosity %.4f > natural %.4f — max-straightness doesn't straighten paths",
			straight, natural)
	}
}

// TestElevationRange verifies the invariants of the elevation field:
// land cells > 0, water cells ≤ 0, and everything within [-1, 1].
func TestElevationRange(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 8; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, c := range tm.Cells {
			if c.Elevation < -1.01 || c.Elevation > 1.01 {
				t.Errorf("seed %d: cell %d elevation %.3f outside [-1,1]",
					cfg.Seed, c.ID, c.Elevation)
			}
			if c.Terrain == "land" && c.Elevation <= 0 {
				t.Errorf("seed %d: land cell %d has non-positive elevation %.3f",
					cfg.Seed, c.ID, c.Elevation)
			}
			if c.Terrain == "water" && c.Elevation > 0 {
				t.Errorf("seed %d: water cell %d has positive elevation %.3f",
					cfg.Seed, c.ID, c.Elevation)
			}
		}
	}
}

// TestPitFillGuaranteesDownhillPath verifies that after pit-filling, every
// land cell has at least one neighbor with strictly-lower elevation. Without
// this property, rivers would get stuck in basins.
func TestPitFillGuaranteesDownhillPath(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		neighbors := make(map[int][]int, len(tm.Cells))
		for _, e := range tm.Edges {
			neighbors[e.Cells[0]] = append(neighbors[e.Cells[0]], e.Cells[1])
			neighbors[e.Cells[1]] = append(neighbors[e.Cells[1]], e.Cells[0])
		}
		var stuck int
		for _, c := range tm.Cells {
			if c.Terrain != "land" {
				continue
			}
			hasDown := false
			for _, nb := range neighbors[c.ID] {
				if tm.Cells[nb].Elevation < c.Elevation {
					hasDown = true
					break
				}
			}
			if !hasDown {
				stuck++
			}
		}
		if stuck > 0 {
			t.Errorf("seed %d: %d land cells have no downhill neighbor (pit-fill incomplete)",
				cfg.Seed, stuck)
		}
	}
}

// TestWaterBathymetryCoastToDeep verifies the frontend's depth-shading assumption:
// water cells adjacent to land have higher elevation (closer to 0) than
// deep-water cells (more negative).
func TestWaterBathymetryCoastToDeep(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 5; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.LakesEnabled = false
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		neighbors := make(map[int][]int, len(tm.Cells))
		for _, e := range tm.Edges {
			neighbors[e.Cells[0]] = append(neighbors[e.Cells[0]], e.Cells[1])
			neighbors[e.Cells[1]] = append(neighbors[e.Cells[1]], e.Cells[0])
		}
		// For every water cell: any water neighbor farther from land should have
		// lower (more negative) elevation, or equal if tied.
		// Simpler spot check: shoreline water cells (adjacent to land) should
		// have higher elevation than non-shoreline water cells on average.
		var shoreSum, deepSum float64
		var shoreN, deepN int
		for _, c := range tm.Cells {
			if c.Terrain != "water" {
				continue
			}
			shore := false
			for _, nb := range neighbors[c.ID] {
				if tm.Cells[nb].Terrain == "land" {
					shore = true
					break
				}
			}
			if shore {
				shoreSum += c.Elevation
				shoreN++
			} else {
				deepSum += c.Elevation
				deepN++
			}
		}
		if shoreN == 0 || deepN == 0 {
			continue
		}
		shoreAvg := shoreSum / float64(shoreN)
		deepAvg := deepSum / float64(deepN)
		if shoreAvg <= deepAvg {
			t.Errorf("seed %d: shore avg %.3f ≤ deep avg %.3f — bathymetry inverted",
				cfg.Seed, shoreAvg, deepAvg)
		}
	}
}

// TestTributaryMergesIntoRiver verifies that when multiple rivers are routed,
// a later river can terminate at a cell belonging to an earlier river
// (tributary merge). Confirms the tributaryBonus attractor works.
func TestTributaryMergesIntoRiver(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var mergedSeeds, totalSeeds int
	for i := 0; i < 20; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.LakesEnabled = false
		// Four rivers in a small map → high chance of tributary encounters.
		cfg.Terrain.Rivers = []RiverSpec{
			{Width: "wide", Origin: "border", End: "coast"},
			{Width: "medium", Origin: "border", End: "coast"},
			{Width: "medium", Origin: "border", End: "coast"},
			{Width: "narrow", Origin: "border", End: "coast"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Rivers) < 2 {
			continue
		}
		totalSeeds++
		// Check if any river's MOUTH cell is already a river cell from another
		// river (merged junction).
		riverCellToRiverID := make(map[int]int)
		for _, rv := range tm.Rivers {
			for i, cid := range rv.CellPath {
				if i == len(rv.CellPath)-1 {
					continue // don't count own mouth
				}
				riverCellToRiverID[cid] = rv.ID
			}
		}
		for _, rv := range tm.Rivers {
			mouthCell := rv.CellPath[len(rv.CellPath)-1]
			if otherID, ok := riverCellToRiverID[mouthCell]; ok && otherID != rv.ID {
				mergedSeeds++
				break
			}
		}
	}
	if totalSeeds == 0 {
		t.Skip("not enough multi-river samples")
	}
	if mergedSeeds == 0 {
		t.Errorf("over %d seeds with ≥2 rivers, never merged — tributary routing broken",
			totalSeeds)
	}
}

// TestHighwayAvoidsWaterWhenLandAvailable verifies the A* cost function:
// a highway between two land endpoints should prefer land-only paths when
// possible rather than routing through the sea.
func TestHighwayAvoidsWaterWhenLandAvailable(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 8; i++ {
		cfg := randomConfig(rng.Int63())
		// No coast so the map is all land — forces a pure-land routing test.
		cfg.Terrain.CoastEnabled = false
		cfg.Terrain.LakesEnabled = false
		cfg.Terrain.RiversEnabled = false
		cfg.Terrain.HighwaysEnabled = true
		cfg.Terrain.Highways = []HighwaySpec{
			{From: "north", To: "south"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Highways) == 0 {
			continue
		}
		for _, hw := range tm.Highways {
			for _, cid := range hw.CellPath {
				if tm.Cells[cid].Terrain == "water" {
					t.Errorf("seed %d: highway %d passes through water cell %d on all-land map",
						cfg.Seed, hw.ID, cid)
				}
			}
		}
	}
}

// TestVertexElevationsShared verifies shared vertices across cells get the
// same VertexElevations value — clients relying on these for smooth meshes
// need the elevation at each shared corner to match exactly.
func TestVertexElevationsShared(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 5; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		type vkey struct{ x, y int64 }
		keyFor := func(p Point) vkey {
			return vkey{int64(math.Round(p.X * 100)), int64(math.Round(p.Y * 100))}
		}
		observed := make(map[vkey]float64)
		for _, c := range tm.Cells {
			if len(c.VertexElevations) != len(c.Vertices) {
				t.Errorf("seed %d: cell %d has %d vertices but %d elevations",
					cfg.Seed, c.ID, len(c.Vertices), len(c.VertexElevations))
				continue
			}
			for j, v := range c.Vertices {
				k := keyFor(v)
				got := c.VertexElevations[j]
				if prev, seen := observed[k]; seen {
					if math.Abs(prev-got) > 1e-9 {
						t.Errorf("seed %d: vertex %v elevation inconsistent: %.6f vs %.6f",
							cfg.Seed, k, prev, got)
					}
				} else {
					observed[k] = got
				}
			}
		}
	}
}

// TestSchemaVersionSet verifies meta.schemaVersion matches the constant.
func TestSchemaVersionSet(t *testing.T) {
	cfg := baseConfig()
	tm, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Meta.SchemaVersion != SchemaVersion {
		t.Errorf("meta.schemaVersion: want %q, got %q", SchemaVersion, tm.Meta.SchemaVersion)
	}
}

// TestRiverCurveWellFormed verifies every river curve has at least 2 points,
// its source-side endpoint reaches the map border when origin=border, and
// its mouth endpoint matches the terminus cell center.
func TestRiverCurveWellFormed(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		const margin = 2.0 // projectToBorder output must be ON the border
		for _, rv := range tm.Rivers {
			if len(rv.Curve) < 2 {
				t.Errorf("seed %d: river %d curve too short (%d)", cfg.Seed, rv.ID, len(rv.Curve))
				continue
			}
			// Inland-origin rivers: curve[0] = source. Border-origin + near
			// border: curve[0] projects onto the map edge.
			first := rv.Curve[0]
			spec := cfg.Terrain.Rivers[rv.ID]
			if spec.Origin == "border" && nearBorder(rv.Source, w, h, 40) {
				onBorder := first.X <= margin || first.X >= w-margin ||
					first.Y <= margin || first.Y >= h-margin
				if !onBorder {
					t.Errorf("seed %d: border-origin river %d curve[0] (%.0f,%.0f) not on map border",
						cfg.Seed, rv.ID, first.X, first.Y)
				}
			} else if first != rv.Source {
				t.Errorf("seed %d: river %d curve[0] %v != source %v",
					cfg.Seed, rv.ID, first, rv.Source)
			}
			// Non-offmap rivers terminate exactly at mouth cell center.
			last := rv.Curve[len(rv.Curve)-1]
			if spec.End != "offmap" && last != rv.Mouth {
				t.Errorf("seed %d: river %d curve[end] %v != mouth %v",
					cfg.Seed, rv.ID, last, rv.Mouth)
			}
		}
	}
}

// TestHighwayCurveWellFormed verifies highway curves reach the map border
// on both ends (highways conceptually span border-to-border).
func TestHighwayCurveWellFormed(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 5; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.HighwaysEnabled = true
		cfg.Terrain.Highways = []HighwaySpec{
			{From: "north", To: "south"},
			{From: "west", To: "east"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		const margin = 2.0
		for _, hw := range tm.Highways {
			if len(hw.Curve) < 2 {
				t.Errorf("seed %d: highway %d curve too short (%d)", cfg.Seed, hw.ID, len(hw.Curve))
				continue
			}
			first := hw.Curve[0]
			last := hw.Curve[len(hw.Curve)-1]
			onBorderStart := first.X <= margin || first.X >= w-margin ||
				first.Y <= margin || first.Y >= h-margin
			onBorderEnd := last.X <= margin || last.X >= w-margin ||
				last.Y <= margin || last.Y >= h-margin
			// At least one end should land on the map border (coastal termini
			// inland do not; handled below).
			if nearBorder(hw.From, w, h, 40) && !onBorderStart {
				t.Errorf("seed %d: highway %d curve[0] (%.0f,%.0f) not on border but From is near border",
					cfg.Seed, hw.ID, first.X, first.Y)
			}
			if nearBorder(hw.To, w, h, 40) && !onBorderEnd {
				t.Errorf("seed %d: highway %d curve[end] (%.0f,%.0f) not on border but To is near border",
					cfg.Seed, hw.ID, last.X, last.Y)
			}
		}
	}
}

// TestBiomesAssigned verifies every cell receives a non-empty biome value
// from the allowed enum, and that biome correlates with terrain as expected:
// water cells → water biomes, land cells → land biomes.
func TestBiomesAssigned(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	waterBiomes := map[string]bool{
		BiomeOcean: true, BiomeCoast: true, BiomeLake: true, BiomeRiver: true,
	}
	landBiomes := map[string]bool{
		BiomeBeach: true, BiomeGrassland: true, BiomeHills: true,
		BiomeMountain: true, BiomePeak: true,
	}
	for i := 0; i < 10; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, c := range tm.Cells {
			if c.Biome == "" {
				t.Errorf("seed %d: cell %d has empty biome", cfg.Seed, c.ID)
				continue
			}
			if c.Terrain == "water" && !waterBiomes[c.Biome] {
				t.Errorf("seed %d: water cell %d has land biome %q",
					cfg.Seed, c.ID, c.Biome)
			}
			if c.Terrain == "land" && !landBiomes[c.Biome] {
				t.Errorf("seed %d: land cell %d has water biome %q",
					cfg.Seed, c.ID, c.Biome)
			}
		}
	}
}

// TestBiomeRiverLakeFlags verifies biome matches the cell's River/Lake flags.
func TestBiomeRiverLakeFlags(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 5; i++ {
		cfg := randomConfig(rng.Int63())
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		for _, c := range tm.Cells {
			if c.River && c.Biome != BiomeRiver {
				t.Errorf("seed %d: cell %d has River=true but biome=%q",
					cfg.Seed, c.ID, c.Biome)
			}
			if c.Lake && c.Biome != BiomeLake {
				t.Errorf("seed %d: cell %d has Lake=true but biome=%q",
					cfg.Seed, c.ID, c.Biome)
			}
		}
	}
}

// TestWorldScaleEcho verifies the configured WorldScale appears in Meta
// and defaults to 1.0 when unset.
func TestWorldScaleEcho(t *testing.T) {
	cfg := randomConfig(42)
	tm, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Meta.WorldScale != 1.0 {
		t.Errorf("default WorldScale: want 1.0, got %v", tm.Meta.WorldScale)
	}
	cfg2 := randomConfig(42)
	cfg2.WorldScale = 2.5
	tm2, err := Generate(cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if tm2.Meta.WorldScale != 2.5 {
		t.Errorf("explicit WorldScale=2.5 not echoed: got %v", tm2.Meta.WorldScale)
	}
}

// TestHighwaysConnectBorders verifies A*-routed highways start at the
// configured "from" border and end at the configured "to" border.
func TestHighwaysConnectBorders(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 8; i++ {
		cfg := randomConfig(rng.Int63())
		cfg.Terrain.HighwaysEnabled = true
		cfg.Terrain.Highways = []HighwaySpec{
			{From: "north", To: "south"},
			{From: "west", To: "east"},
		}
		tm, err := Generate(cfg)
		if err != nil {
			t.Fatalf("seed %d: %v", cfg.Seed, err)
		}
		if len(tm.Highways) != len(cfg.Terrain.Highways) {
			t.Errorf("seed %d: configured %d highways, generated %d",
				cfg.Seed, len(cfg.Terrain.Highways), len(tm.Highways))
		}
		w, h := float64(cfg.Width), float64(cfg.Height)
		const margin = 40.0
		cellMap := make(map[int]*Cell, len(tm.Cells))
		for i := range tm.Cells {
			cellMap[tm.Cells[i].ID] = &tm.Cells[i]
		}
		// Endpoint is valid if it's at the configured border OR it's a
		// coastline land cell on the corresponding map half (fallback when
		// that whole side is coast water).
		checkEndpoint := func(hwID int, endName string, pt Point, side string) {
			atBorder := false
			onHalf := false
			switch side {
			case "north":
				atBorder = pt.Y <= margin
				onHalf = pt.Y < h*0.5
			case "south":
				atBorder = pt.Y >= h-margin
				onHalf = pt.Y > h*0.5
			case "east":
				atBorder = pt.X >= w-margin
				onHalf = pt.X > w*0.5
			case "west":
				atBorder = pt.X <= margin
				onHalf = pt.X < w*0.5
			}
			if atBorder {
				return
			}
			// Fallback: coastline cell on the right half.
			for _, c := range cellMap {
				if c.Center.X != pt.X || c.Center.Y != pt.Y {
					continue
				}
				isCoastal := c.Terrain == "land"
				if isCoastal {
					hasWaterNb := false
					for _, nb := range c.Neighbors {
						if cellMap[nb].Terrain == "water" {
							hasWaterNb = true
							break
						}
					}
					isCoastal = hasWaterNb
				}
				if onHalf && isCoastal {
					return
				}
			}
			t.Errorf("seed %d: highway %d %s (%.0f,%.0f) not at %s border or coast",
				cfg.Seed, hwID, endName, pt.X, pt.Y, side)
		}
		for _, hw := range tm.Highways {
			spec := cfg.Terrain.Highways[hw.ID]
			checkEndpoint(hw.ID, "from", hw.From, spec.From)
			checkEndpoint(hw.ID, "to", hw.To, spec.To)
			if len(hw.CellPath) < 2 {
				t.Errorf("seed %d: highway %d path too short: %d cells",
					cfg.Seed, hw.ID, len(hw.CellPath))
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
