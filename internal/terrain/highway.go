package terrain

import (
	"container/heap"
	"math"
	"math/rand"
)

// generateHighways runs A* per HighwaySpec to connect its From border to
// its To border, minimizing a cost function that penalizes steep slopes
// and water crossings. Existing highway cells are cheaper to reuse so
// networks of highways naturally share segments at interchanges.
func generateHighways(cells []Cell, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []Highway {
	w, h := float64(cfg.Width), float64(cfg.Height)
	used := make(map[int]bool)
	var highways []Highway

	for id, spec := range cfg.Terrain.Highways {
		from := spec.From
		to := spec.To
		if from == "" {
			from = "north"
		}
		if to == "" {
			to = oppositeSide(from)
		}

		fromPool := highwayBorderCells(cells, from, w, h)
		toPool := highwayBorderCells(cells, to, w, h)
		if len(fromPool) == 0 || len(toPool) == 0 {
			continue
		}

		src := fromPool[rng.Intn(len(fromPool))]
		dst := toPool[rng.Intn(len(toPool))]

		path := routeHighway(src, dst, cells, diag.neighbors, used)
		if len(path) < 2 {
			continue
		}
		for _, cid := range path {
			used[cid] = true
		}

		width := spec.Width
		if width == "" {
			width = "medium"
		}
		highways = append(highways, Highway{
			ID:       id,
			CellPath: path,
			From:     cells[path[0]].Center,
			To:       cells[path[len(path)-1]].Center,
			Width:    width,
		})
	}
	return highways
}

func oppositeSide(s string) string {
	switch s {
	case "north":
		return "south"
	case "south":
		return "north"
	case "east":
		return "west"
	case "west":
		return "east"
	}
	return "south"
}

// highwayBorderCells returns cells sitting within a small margin of the
// given side. Land cells are preferred, but water cells are accepted as
// fallback so a highway whose target side is the coast (e.g. from=N to=S
// with coast=south) can still terminate — it just ends at the waterfront
// and the render extends the stroke out to the map edge.
func highwayBorderCells(cells []Cell, side string, w, h float64) []int {
	const margin = 35.0
	var land, water []int
	for _, c := range cells {
		cx, cy := c.Center.X, c.Center.Y
		match := false
		switch side {
		case "north":
			match = cy <= margin
		case "south":
			match = cy >= h-margin
		case "east":
			match = cx >= w-margin
		case "west":
			match = cx <= margin
		}
		if !match {
			continue
		}
		if c.Terrain == "land" {
			land = append(land, c.ID)
		} else {
			water = append(water, c.ID)
		}
	}
	if len(land) > 0 {
		return land
	}
	return water
}

// routeHighway returns the A*-optimal cell path from src to dst, or nil
// if no path exists (should not happen on a connected Voronoi graph).
func routeHighway(src, dst int, cells []Cell, neighbors [][]int, used map[int]bool) []int {
	target := cells[dst].Center
	heuristic := func(id int) float64 {
		return math.Hypot(cells[id].Center.X-target.X, cells[id].Center.Y-target.Y)
	}
	edgeCost := func(a, b int) float64 {
		ca, cb := cells[a], cells[b]
		dx := cb.Center.X - ca.Center.X
		dy := cb.Center.Y - ca.Center.Y
		dist := math.Hypot(dx, dy)
		cost := dist
		// Steep terrain: elevation change scaled by distance.
		dElev := math.Abs(cb.Elevation - ca.Elevation)
		cost += dElev * dist * 8
		// Crossing into water (bridge): ~4× the normal segment cost.
		if cb.Terrain == "water" {
			cost += dist * 3
		}
		// Merging onto an existing highway is cheap — natural interchanges.
		if used[b] {
			cost *= 0.5
		}
		return cost
	}

	gScore := map[int]float64{src: 0}
	cameFrom := map[int]int{}
	closed := map[int]bool{}

	open := &hwPQ{}
	heap.Push(open, &hwNode{id: src, f: heuristic(src)})

	for open.Len() > 0 {
		cur := heap.Pop(open).(*hwNode)
		if closed[cur.id] {
			continue
		}
		if cur.id == dst {
			path := []int{dst}
			n := dst
			for n != src {
				n = cameFrom[n]
				path = append([]int{n}, path...)
			}
			return path
		}
		closed[cur.id] = true
		for _, nb := range neighbors[cur.id] {
			if closed[nb] {
				continue
			}
			g := gScore[cur.id] + edgeCost(cur.id, nb)
			if gs, seen := gScore[nb]; !seen || g < gs {
				gScore[nb] = g
				cameFrom[nb] = cur.id
				heap.Push(open, &hwNode{id: nb, f: g + heuristic(nb)})
			}
		}
	}
	return nil
}

// ── A* priority queue ────────────────────────────────────────────────────────

type hwNode struct {
	id int
	f  float64
}

type hwPQ []*hwNode

func (pq hwPQ) Len() int            { return len(pq) }
func (pq hwPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq hwPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *hwPQ) Push(x interface{}) { *pq = append(*pq, x.(*hwNode)) }
func (pq *hwPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}
