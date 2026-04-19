package terrain

import "github.com/corpllm/mapgen/internal/noise"

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

		// Large-scale mountain ranges + small-scale detail.
		mountains := pn.Eval01(cx/w*3, cy/h*3)
		detail := pn.Eval01(cx/w*8, cy/h*8)

		elev := gradient*0.55 + mountains*0.35 + detail*0.10
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
