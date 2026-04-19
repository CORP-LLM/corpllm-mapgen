package terrain

import (
	"math"
	"math/rand"
)

// generateRivers routes one river per RiverSpec. Each spec has its own
// origin (border/inland), end (coast/inland), and width.
func generateRivers(cells []Cell, edges []Edge, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []River {
	w := float64(cfg.Width)
	h := float64(cfg.Height)
	borderDist := math.Min(w, h) * 0.15
	inlandDist := math.Min(w, h) * 0.3

	// Edge lookup for assembling the river path.
	edgeIndex := make(map[[2]int]int, len(diag.edgePairs))
	for i, p := range diag.edgePairs {
		a, b := p[0], p[1]
		if a > b {
			a, b = b, a
		}
		edgeIndex[[2]int{a, b}] = i
	}

	// Partition land cells by zone once; re-filter per spec to exclude used cells.
	var borderPool, inlandPool []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
		cx, cy := c.Center.X, c.Center.Y
		atBorder := cx < borderDist || cx > w-borderDist || cy < borderDist || cy > h-borderDist
		atInland := cx >= inlandDist && cx <= w-inlandDist && cy >= inlandDist && cy <= h-inlandDist
		if atBorder {
			borderPool = append(borderPool, c.ID)
		}
		if atInland {
			inlandPool = append(inlandPool, c.ID)
		}
	}

	var rivers []River
	used := make(map[int]bool)
	targetInlandLen := math.Hypot(w, h) * 0.25

	for id, spec := range cfg.Terrain.Rivers {
		// Pool by origin.
		pool := borderPool
		if spec.Origin == "inland" {
			pool = inlandPool
		}
		// Pick unused random candidate.
		var srcID int = -1
		for attempts := 0; attempts < 30 && srcID == -1 && len(pool) > 0; attempts++ {
			idx := rng.Intn(len(pool))
			cand := pool[idx]
			if !used[cand] && cells[cand].Terrain == "land" {
				srcID = cand
			}
			pool = append(pool[:idx], pool[idx+1:]...)
		}
		if srcID == -1 {
			continue
		}

		// Target direction: used by straightness to prefer forward-aligned moves.
		var tx, ty float64
		if spec.End == "coast" {
			switch cfg.Terrain.CoastSide {
			case "north":
				ty = -1
			case "south":
				ty = 1
			case "east":
				tx = 1
			case "west":
				tx = -1
			}
		} else {
			// Inland: aim toward map center.
			src := cells[srcID].Center
			dx, dy := w/2-src.X, h/2-src.Y
			l := math.Hypot(dx, dy)
			if l > 0 {
				tx, ty = dx/l, dy/l
			}
		}

		path := routeRiver(srcID, cells, diag.neighbors, w, h, used, spec.End, targetInlandLen, spec.Straightness, tx, ty)
		if len(path) < 2 {
			continue
		}

		// Edge IDs.
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

		for _, cid := range path {
			cells[cid].River = true
			cells[cid].Terrain = "water"
			used[cid] = true
		}

		width := spec.Width
		if width == "" {
			width = "medium"
		}
		rivers = append(rivers, River{
			ID:     id,
			Path:   edgePath,
			Source: cells[path[0]].Center,
			Mouth:  cells[path[len(path)-1]].Center,
			Width:  width,
		})
	}
	return rivers
}

// routeRiver greedy-routes from srcID. Termination depends on end:
//   - "coast":   stop when water is reached
//   - "inland":  stop after ~targetLen Euclidean distance from source
//
// straightness (0–1) biases each step toward the (tx,ty) direction. Straightness=1
// produces a nearly straight channel — cyberpunk canals rather than natural rivers.
func routeRiver(srcID int, cells []Cell, neighbors [][]int, w, h float64,
	used map[int]bool, end string, targetLen float64,
	straightness, tx, ty float64) []int {
	const maxSteps = 200
	path := []int{srcID}
	visited := map[int]bool{srcID: true}
	cur := srcID
	src := cells[srcID].Center

	maxDim := math.Max(w, h)
	alignWeight := straightness * maxDim * 1.5

	for step := 0; step < maxSteps; step++ {
		c := cells[cur]

		if end == "coast" && c.Terrain == "water" {
			break
		}
		if end == "inland" {
			d := math.Hypot(c.Center.X-src.X, c.Center.Y-src.Y)
			if d >= targetLen && step >= 3 {
				break
			}
			if c.Terrain == "water" {
				break
			}
		}

		best := -1
		bestScore := math.MaxFloat64
		curCenter := c.Center
		for _, nb := range neighbors[cur] {
			if visited[nb] {
				continue
			}
			nc := cells[nb]
			var score float64
			switch end {
			case "inland":
				d := math.Hypot(nc.Center.X-src.X, nc.Center.Y-src.Y)
				score = -d + (1 / (distToEdge(nc.Center, w, h) + 1))
				if nc.Terrain == "water" {
					score += 1e6
				}
			default: // "coast"
				score = distToEdge(nc.Center, w, h)
				if nc.Terrain == "water" {
					score -= 1e9
				}
			}
			if used[nb] {
				score += 500
			}
			// Straightness bias: reward neighbors aligned with target direction.
			if alignWeight > 0 {
				mx := nc.Center.X - curCenter.X
				my := nc.Center.Y - curCenter.Y
				ml := math.Hypot(mx, my)
				if ml > 0 {
					align := (mx*tx + my*ty) / ml
					score -= align * alignWeight
				}
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
