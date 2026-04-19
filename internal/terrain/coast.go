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
