package terrain

// assignSuitability writes a [0,1] score per cell indicating how good the
// location is for urban development. Inputs: biome, elevation, nearby water
// (rivers/coast → resources), nearby highway (transit access).
//
// Water cells and rivers score 0 — unbuildable. Grassland near both water
// and a highway scores near 1.0. Mountains/peaks score low. The CityGraph
// generator in corpllm-server consumes this to decide where sectors fit.
func assignSuitability(cells []Cell, neighbors [][]int, highways []Highway) {
	// Build the set of highway cells once for fast lookup.
	isHighway := make(map[int]bool)
	for _, hw := range highways {
		for _, cid := range hw.CellPath {
			isHighway[cid] = true
		}
	}

	// Base score by biome — the starting point before neighbor adjustments.
	baseScore := func(biome string) float64 {
		switch biome {
		case BiomeGrassland:
			return 0.75
		case BiomeHills:
			return 0.55
		case BiomeBeach:
			return 0.25 // sandy, unstable ground
		case BiomeMountain:
			return 0.20
		case BiomePeak:
			return 0.05
		}
		return 0 // water biomes
	}

	for i := range cells {
		c := &cells[i]
		// Water and river-marked cells are off-limits regardless of biome.
		if c.Terrain != "land" || c.River {
			c.Suitability = 0
			continue
		}
		score := baseScore(c.Biome)

		// Adjacency bonuses — clients want cities near resources and transit.
		nearCoastWater := false
		nearRiver := false
		nearHighway := false
		for _, nbID := range neighbors[i] {
			nb := cells[nbID]
			if nb.Biome == BiomeCoast {
				nearCoastWater = true
			}
			if nb.River {
				nearRiver = true
			}
			if isHighway[nbID] {
				nearHighway = true
			}
		}
		if nearCoastWater {
			score += 0.15
		}
		if nearRiver {
			score += 0.10
		}
		if nearHighway {
			score += 0.10
		}

		// Slope penalty: steep gradients are hard to build on. Measure as the
		// max elevation gap to any land neighbor.
		var maxGap float64
		for _, nbID := range neighbors[i] {
			nb := cells[nbID]
			if nb.Terrain != "land" {
				continue
			}
			d := nb.Elevation - c.Elevation
			if d < 0 {
				d = -d
			}
			if d > maxGap {
				maxGap = d
			}
		}
		// Typical gap on hilly terrain: 0.02–0.10. Over 0.08 starts to hurt.
		if maxGap > 0.08 {
			score -= (maxGap - 0.08) * 2
		}

		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		c.Suitability = score
	}
}
