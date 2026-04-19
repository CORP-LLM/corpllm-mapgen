package terrain

import (
	"math"
	"math/rand"
)

// generateRivers routes rivers from land border cells toward water/edge.
func generateRivers(cells []Cell, edges []Edge, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []River {
	w := float64(cfg.Width)
	h := float64(cfg.Height)
	borderDist := math.Min(w, h) * 0.15

	// Build edge lookup: given a cell pair, find edge index.
	edgeIndex := make(map[[2]int]int, len(diag.edgePairs))
	for i, p := range diag.edgePairs {
		a, b := p[0], p[1]
		if a > b {
			a, b = b, a
		}
		edgeIndex[[2]int{a, b}] = i
	}

	// Candidate source cells: land, near map border.
	var candidates []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
		cx, cy := c.Center.X, c.Center.Y
		if cx < borderDist || cx > w-borderDist || cy < borderDist || cy > h-borderDist {
			candidates = append(candidates, c.ID)
		}
	}

	var rivers []River
	used := make(map[int]bool)

	for id := 0; id < cfg.Terrain.RiverCount && len(candidates) > 0; id++ {
		// Pick random unused candidate.
		idx := rng.Intn(len(candidates))
		srcID := candidates[idx]
		candidates = append(candidates[:idx], candidates[idx+1:]...)
		if used[srcID] {
			continue
		}

		path := routeRiver(srcID, cells, diag.neighbors, w, h, used)
		if len(path) < 2 {
			continue
		}

		// Collect edge IDs along path.
		edgePath := make([]int, 0, len(path)-1)
		for k := 0; k < len(path)-1; k++ {
			a, b := path[k], path[k+1]
			if a > b {
				a, b = b, a
			}
			if ei, ok := edgeIndex[[2]int{a, b}]; ok {
				edgePath = append(edgePath, ei)
				edges[ei].River = true
			}
		}
		if len(edgePath) == 0 {
			continue
		}

		// Mark cells.
		for _, cid := range path {
			cells[cid].River = true
			cells[cid].Terrain = "water"
			used[cid] = true
		}

		src := cells[path[0]].Center
		mouth := cells[path[len(path)-1]].Center
		rivers = append(rivers, River{
			ID:     id,
			Path:   edgePath,
			Source: src,
			Mouth:  mouth,
			Width:  cfg.Terrain.RiverWidth,
		})
	}
	return rivers
}

// routeRiver does greedy routing from srcID toward water/edge.
func routeRiver(srcID int, cells []Cell, neighbors [][]int, w, h float64, used map[int]bool) []int {
	const maxSteps = 200
	path := []int{srcID}
	visited := map[int]bool{srcID: true}
	cur := srcID

	for step := 0; step < maxSteps; step++ {
		c := cells[cur]
		// Reached water — done.
		if c.Terrain == "water" {
			break
		}

		best := -1
		bestScore := math.MaxFloat64

		for _, nb := range neighbors[cur] {
			if visited[nb] {
				continue
			}
			nc := cells[nb]
			// Score: prefer water, then cells closer to map edge.
			score := distToEdge(nc.Center, w, h)
			if nc.Terrain == "water" {
				score -= 1e9
			}
			if used[nb] {
				score += 500
			}
			if score < bestScore {
				bestScore = score
				best = nb
			}
		}
		if best == -1 {
			break
		}
		path = append(path, best)
		visited[best] = true
		cur = best
	}
	return path
}

func distToEdge(p Point, w, h float64) float64 {
	d := p.X
	if p.Y < d {
		d = p.Y
	}
	if w-p.X < d {
		d = w - p.X
	}
	if h-p.Y < d {
		d = h - p.Y
	}
	return d
}
