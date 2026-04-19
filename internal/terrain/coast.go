package terrain

import (
	"math/rand"
	"sort"

	"github.com/corpllm/mapgen/internal/noise"
)

// assignCoast uses Perlin noise + directional gradient to set water/land.
func assignCoast(cells []Cell, cfg *Config, _ *rand.Rand) {
	pn := noise.New(cfg.Seed ^ 0xDEADBEEF)
	w := float64(cfg.Width)
	h := float64(cfg.Height)

	scale := 4.0 // noise frequency

	type scored struct {
		idx   int
		score float64
	}
	scored_ := make([]scored, len(cells))

	for i, c := range cells {
		nx := c.Center.X / w * scale
		ny := c.Center.Y / h * scale
		noiseVal := pn.Eval01(nx, ny) // [0,1]

		// Directional gradient: value 0 at the coast side, 1 at opposite side.
		var gradient float64
		switch cfg.Terrain.CoastSide {
		case "south":
			gradient = 1 - c.Center.Y/h
		case "north":
			gradient = c.Center.Y / h
		case "west":
			gradient = c.Center.X / w
		case "east":
			gradient = 1 - c.Center.X/w
		default:
			gradient = 1 // all land when "none"
		}

		// Blend gradient and noise.
		blend := gradient*(1-cfg.Terrain.CoastNoise) + noiseVal*cfg.Terrain.CoastNoise
		// Hard cutoff: cells too far inland can never become water regardless of noise.
		inlandCutoff := cfg.Terrain.WaterRatio + 0.3
		if inlandCutoff > 0.75 {
			inlandCutoff = 0.75
		}
		if gradient > inlandCutoff {
			blend = 1.0
		}
		scored_[i] = scored{i, blend}
	}

	// Sort by score; bottom waterRatio fraction becomes water.
	sort.Slice(scored_, func(a, b int) bool {
		return scored_[a].score < scored_[b].score
	})
	waterCount := int(float64(len(cells)) * cfg.Terrain.WaterRatio)
	for i := 0; i < waterCount; i++ {
		cells[scored_[i].idx].Terrain = "water"
	}
}

// removeInlandWater reverts any water cell that is not reachable from a
// map-border water cell via water neighbors. Prevents noise from creating
// isolated inland lakes during coast assignment.
func removeInlandWater(cells []Cell, diag *voronoiDiagram, cfg *Config) {
	w := float64(cfg.Width)
	h := float64(cfg.Height)
	const margin = 2.0

	seen := make([]bool, len(cells))
	queue := make([]int, 0, len(cells)/4)

	for i := range cells {
		if cells[i].Terrain != "water" {
			continue
		}
		for _, v := range cells[i].Vertices {
			if v.X <= margin || v.X >= w-margin ||
				v.Y <= margin || v.Y >= h-margin {
				seen[i] = true
				queue = append(queue, i)
				break
			}
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range diag.neighbors[cur] {
			if seen[nb] || cells[nb].Terrain != "water" {
				continue
			}
			seen[nb] = true
			queue = append(queue, nb)
		}
	}

	for i := range cells {
		if cells[i].Terrain == "water" && !seen[i] {
			cells[i].Terrain = "land"
		}
	}
}
