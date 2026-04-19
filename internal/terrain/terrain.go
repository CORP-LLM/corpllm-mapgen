package terrain

import (
	"fmt"
	"math/rand"
	"time"
)

// Generate runs the full pipeline and returns a Terrain.
func Generate(cfg *Config) (*Terrain, error) {
	cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(cfg.Seed))
	w := float64(cfg.Width)
	h := float64(cfg.Height)

	// 1. Generate sites.
	sites := generateSites(cfg.CellCount, w, h, rng)

	// 2+3. Build Voronoi + Lloyd relaxation.
	for i := 0; i < cfg.RelaxIterations; i++ {
		sites = lloydRelax(sites, w, h)
	}
	diag := buildVoronoi(sites, w, h)

	// 4. Build cells.
	cells := make([]Cell, len(sites))
	for i, s := range sites {
		cells[i] = Cell{
			ID:        i,
			Center:    s,
			Vertices:  diag.cellVerts[i],
			Terrain:   "land",
			Neighbors: diag.neighbors[i],
		}
	}

	// 5. Terrain assignment.
	if cfg.Terrain.CoastEnabled {
		assignCoast(cells, cfg, rng)
		removeInlandWater(cells, diag, cfg)
	}

	// 5b. Elevation (after coast so water cells are known = 0 height).
	assignElevation(cells, cfg)
	fillPits(cells, diag.neighbors)
	assignWaterDepth(cells, diag.neighbors)

	// 6. Build edges.
	edges := buildEdges(diag, cells)

	// 7. Lakes first — rivers may target them when end="lake".
	var lakes []Lake
	if cfg.Terrain.LakesEnabled && len(cfg.Terrain.Lakes) > 0 {
		lakes = generateLakes(cells, diag, cfg, rng)
	}

	// 8. Rivers.
	var rivers []River
	if cfg.Terrain.RiversEnabled && len(cfg.Terrain.Rivers) > 0 {
		rivers = generateRivers(cells, edges, diag, cfg, rng)
	}

	// Rivers and lakes turned land cells into water — zero their elevation
	// (water surface ≈ 0) and recompute bathymetry so depth is consistent
	// with the final water layout.
	for i := range cells {
		if cells[i].Terrain == "water" && cells[i].Elevation > 0 {
			cells[i].Elevation = 0
		}
	}
	assignWaterDepth(cells, diag.neighbors)

	// Rebuild edges after all water assignment (rivers + lakes changed cell terrain).
	edges = buildEdges(diag, cells)
	// Re-mark river edges lost during edge rebuild.
	for _, r := range rivers {
		for _, eid := range r.Path {
			if eid >= 0 && eid < len(edges) {
				edges[eid].River = true
			}
		}
	}

	// 9. Highways — A* across the finished cell graph so water and
	// elevation are known when routing.
	var highways []Highway
	if cfg.Terrain.HighwaysEnabled && len(cfg.Terrain.Highways) > 0 {
		highways = generateHighways(cells, diag, cfg, rng)
	}

	// 10. Coastline extraction.
	coastline := extractCoastline(cells, edges)

	// 11. Biomes — derived once all terrain/water assignment is final.
	assignBiomes(cells, diag.neighbors)

	// 12. Per-vertex elevation — averaged across cells sharing each Voronoi
	// vertex so clients can build smooth meshes (no stepped flat-top cells).
	computeVertexElevations(cells)

	id := fmt.Sprintf("t_%08x", rng.Uint32())
	return &Terrain{
		Meta: Meta{
			ID:              id,
			Seed:            cfg.Seed,
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			CellCount:       len(cells),
			RelaxIterations: cfg.RelaxIterations,
			WorldScale:      cfg.WorldScale,
			SchemaVersion:   SchemaVersion,
			Config:          cfg,
		},
		Bounds:    Bounds{Width: cfg.Width, Height: cfg.Height},
		Cells:     cells,
		Edges:     edges,
		Rivers:    rivers,
		Lakes:     lakes,
		Highways:  highways,
		Coastline: coastline,
	}, nil
}

// buildEdges constructs the Edge slice from diagram pairs.
func buildEdges(diag *voronoiDiagram, cells []Cell) []Edge {
	edges := make([]Edge, len(diag.edgePairs))
	for i, pair := range diag.edgePairs {
		a, b := pair[0], pair[1]
		ta := cells[a].Terrain
		tb := cells[b].Terrain
		var t string
		switch {
		case ta == "land" && tb == "land":
			t = "land-land"
		case ta == "water" && tb == "water":
			t = "water-water"
		default:
			t = "land-water"
		}
		edges[i] = Edge{
			ID:       i,
			Cells:    [2]int{a, b},
			Vertices: diag.edges[i],
			Type:     t,
		}
	}
	return edges
}

// extractCoastline marks coastline cells/edges and returns Coastline.
// River and lake borders are excluded — only ocean shorelines are coastline.
func extractCoastline(cells []Cell, edges []Edge) Coastline {
	var coastEdgeIDs []int
	for i := range edges {
		if edges[i].Type != "land-water" {
			continue
		}
		ca, cb := cells[edges[i].Cells[0]], cells[edges[i].Cells[1]]
		if ca.River || cb.River || ca.Lake || cb.Lake {
			continue
		}
		edges[i].Coastline = true
		coastEdgeIDs = append(coastEdgeIDs, edges[i].ID)
		cells[edges[i].Cells[0]].Coastline = true
		cells[edges[i].Cells[1]].Coastline = true
	}
	return Coastline{Edges: coastEdgeIDs, Length: len(coastEdgeIDs)}
}
