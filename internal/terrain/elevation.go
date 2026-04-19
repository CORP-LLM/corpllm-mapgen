package terrain

import (
	"math"

	"github.com/corpllm/mapgen/internal/noise"
)

// assignElevation computes a [0,1] height for every land cell:
// high far from the coast side, low near it, with Perlin variation
// for mountains and valleys. Water cells are clamped to 0 so rivers
// flowing downhill naturally terminate at them.
func assignElevation(cells []Cell, cfg *Config) {
	pn := noise.New(cfg.Seed ^ 0xBADF00D)
	w := float64(cfg.Width)
	h := float64(cfg.Height)

	for i := range cells {
		if cells[i].Terrain == "water" {
			cells[i].Elevation = 0
			continue
		}
		c := &cells[i]
		cx, cy := c.Center.X, c.Center.Y

		var gradient float64
		switch cfg.Terrain.CoastSide {
		case "north":
			gradient = cy / h
		case "south":
			gradient = 1 - cy/h
		case "east":
			gradient = 1 - cx/w
		case "west":
			gradient = cx / w
		default:
			gradient = 0.5
		}

		// Large-scale mountain ranges + small-scale detail. Both are scaled
		// by Roughness so the user can flatten the map: at 0 the terrain is
		// just a smooth coast-distance ramp, at 1 it has full hilly relief.
		r := cfg.Terrain.Roughness
		mountains := pn.Eval01(cx/w*3, cy/h*3) * r
		detail := pn.Eval01(cx/w*8, cy/h*8) * r
		// Re-normalize so total stays in [0,1] regardless of r.
		gradWeight := 1.0 - (0.35+0.10)*r
		elev := gradient*gradWeight + mountains*0.35 + detail*0.10
		if elev < 0 {
			elev = 0
		}
		if elev > 1 {
			elev = 1
		}
		// Minimum elevation for land so it can always drain to water (0).
		if elev < 0.05 {
			elev = 0.05
		}
		c.Elevation = elev
	}
}

// computeVertexElevations writes a per-corner averaged elevation into every
// cell. The average is taken across all cells whose polygons contain the
// same Voronoi vertex (typically 3 cells per vertex). Shared vertices get
// consistent values across cells, so the client's mesh has matching
// elevations at shared corners and renders without seams/steps.
func computeVertexElevations(cells []Cell) {
	type vkey struct{ x, y int64 }
	keyFor := func(p Point) vkey {
		return vkey{
			int64(math.Round(p.X * 100)),
			int64(math.Round(p.Y * 100)),
		}
	}
	buckets := make(map[vkey]struct {
		sum   float64
		count int
	}, len(cells)*4)
	for _, c := range cells {
		for _, v := range c.Vertices {
			k := keyFor(v)
			b := buckets[k]
			b.sum += c.Elevation
			b.count++
			buckets[k] = b
		}
	}
	for i := range cells {
		c := &cells[i]
		c.VertexElevations = make([]float64, len(c.Vertices))
		for j, v := range c.Vertices {
			b := buckets[keyFor(v)]
			if b.count > 0 {
				c.VertexElevations[j] = b.sum / float64(b.count)
			} else {
				c.VertexElevations[j] = c.Elevation
			}
		}
	}
}

// assignWaterDepth computes negative "elevation" for water cells = BFS distance
// from the nearest land cell, normalized to [-1, 0]. Shallow water (coastline)
// is near 0, deep ocean is closer to -1. Reused by the frontend for bathymetry.
func assignWaterDepth(cells []Cell, neighbors [][]int) {
	dist := make([]int, len(cells))
	for i := range dist {
		dist[i] = -1
	}
	var queue []int
	for i := range cells {
		if cells[i].Terrain == "land" {
			dist[i] = 0
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range neighbors[cur] {
			if dist[nb] == -1 && cells[nb].Terrain == "water" {
				dist[nb] = dist[cur] + 1
				queue = append(queue, nb)
			}
		}
	}
	maxD := 0
	for _, d := range dist {
		if d > maxD {
			maxD = d
		}
	}
	if maxD == 0 {
		return
	}
	for i := range cells {
		if cells[i].Terrain == "water" && dist[i] > 0 {
			// Negative: deeper = more negative. Lakes (often just 1–2 from land)
			// stay near the surface; open ocean dives toward -1.
			cells[i].Elevation = -float64(dist[i]) / float64(maxD)
		}
	}
}

// fillPits raises the elevation of local minima so every land cell has a
// strictly-downhill path to water. Without this, Perlin noise produces
// countless little pits where rivers get stuck after 1–2 steps.
//
// Iterative variant of the Planchon–Darboux algorithm: for each land cell,
// if it isn't strictly higher than its lowest neighbor, raise it just above.
// Converges in a handful of iterations for typical maps.
func fillPits(cells []Cell, neighbors [][]int) {
	const eps = 0.002
	const maxIter = 60
	for iter := 0; iter < maxIter; iter++ {
		changed := false
		for i := range cells {
			if cells[i].Terrain == "water" {
				continue
			}
			minNb := math.Inf(1)
			for _, nb := range neighbors[i] {
				if cells[nb].Elevation < minNb {
					minNb = cells[nb].Elevation
				}
			}
			if cells[i].Elevation <= minNb {
				cells[i].Elevation = minNb + eps
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}
