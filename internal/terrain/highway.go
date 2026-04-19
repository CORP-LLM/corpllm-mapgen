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

		fromPool := highwayBorderCells(cells, diag.neighbors, from, w, h)
		toPool := highwayBorderCells(cells, diag.neighbors, to, w, h)
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

// highwayBorderCells picks terminus candidates for the given side:
//
//  1. Land cells within a small margin of the actual map border
//     (normal case — a highway enters/exits at the map edge).
//  2. If the entire side is water (e.g. to=south with coast=south),
//     fall back to coastline LAND cells on that half of the map —
//     a highway ending "at the south" really means ending at the
//     south shore, a waterfront terminus. It does NOT dive into
//     the sea.
func highwayBorderCells(cells []Cell, neighbors [][]int, side string, w, h float64) []int {
	const margin = 35.0
	var land []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
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
		if match {
			land = append(land, c.ID)
		}
	}
	if len(land) > 0 {
		return land
	}

	// Side is entirely water → find coastline land cells on that half.
	var coastal []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
		cx, cy := c.Center.X, c.Center.Y
		onHalf := false
		switch side {
		case "north":
			onHalf = cy < h*0.5
		case "south":
			onHalf = cy > h*0.5
		case "east":
			onHalf = cx > w*0.5
		case "west":
			onHalf = cx < w*0.5
		}
		if !onHalf {
			continue
		}
		for _, nbID := range neighbors[c.ID] {
			if cells[nbID].Terrain == "water" {
				coastal = append(coastal, c.ID)
				break
			}
		}
	}
	return coastal
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
