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
	}

	// 6. Build edges.
	edges := buildEdges(diag, cells)

	// 7. Rivers.
	var rivers []River
	if cfg.Terrain.RiversEnabled && cfg.Terrain.RiverCount > 0 {
		rivers = generateRivers(cells, edges, diag, cfg, rng)
	}

	// 8. Lakes.
	var lakes []Lake
	if cfg.Terrain.LakesEnabled && cfg.Terrain.LakeCount > 0 {
		lakes = generateLakes(cells, diag, cfg, rng)
	}

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

	// 9. Coastline extraction.
	coastline := extractCoastline(cells, edges)

	id := fmt.Sprintf("t_%08x", rng.Uint32())
	return &Terrain{
		Meta: Meta{
			ID:              id,
			Seed:            cfg.Seed,
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			CellCount:       len(cells),
			RelaxIterations: cfg.RelaxIterations,
			Config:          cfg,
		},
		Bounds:    Bounds{Width: cfg.Width, Height: cfg.Height},
		Cells:     cells,
		Edges:     edges,
		Rivers:    rivers,
		Lakes:     lakes,
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
// River cell borders are excluded — rivers are water but not coastline.
func extractCoastline(cells []Cell, edges []Edge) Coastline {
	var coastEdgeIDs []int
	for i := range edges {
		if edges[i].Type != "land-water" {
			continue
		}
		ca, cb := cells[edges[i].Cells[0]], cells[edges[i].Cells[1]]
		if ca.River || cb.River {
			continue
		}
		edges[i].Coastline = true
		coastEdgeIDs = append(coastEdgeIDs, edges[i].ID)
		cells[edges[i].Cells[0]].Coastline = true
		cells[edges[i].Cells[1]].Coastline = true
	}
	return Coastline{Edges: coastEdgeIDs, Length: len(coastEdgeIDs)}
}
