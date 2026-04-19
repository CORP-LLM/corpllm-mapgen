package terrain

// Biome enum — coarse classification the game uses to pick asset categories
// and textures per cell. Derived from terrain + elevation + water adjacency
// AFTER all water assignment (coast, lakes, rivers) and pit-filling are done.
//
// Keep this list in sync with schema/terrain.schema.json and the frontend.
const (
	BiomeOcean     = "ocean"     // deep open water
	BiomeCoast     = "coast"     // shallow sea bordering land
	BiomeLake      = "lake"      // inland still water
	BiomeRiver     = "river"     // flowing water
	BiomeBeach     = "beach"     // land adjacent to ocean/coast
	BiomeGrassland = "grassland" // low flat land
	BiomeHills     = "hills"     // rolling mid-elevation terrain
	BiomeMountain  = "mountain"  // steep rocky terrain
	BiomePeak      = "peak"      // highest snow/rock line
)

// assignBiomes writes a Biome string to every cell. Run after coast, lakes,
// rivers, pit-fill, and water-depth so all inputs are final.
func assignBiomes(cells []Cell, neighbors [][]int) {
	for i := range cells {
		c := &cells[i]
		if c.Terrain == "water" {
			switch {
			case c.River:
				c.Biome = BiomeRiver
			case c.Lake:
				c.Biome = BiomeLake
			default:
				// Coast if any neighbor is land; otherwise open ocean.
				coast := false
				for _, nb := range neighbors[i] {
					if cells[nb].Terrain == "land" {
						coast = true
						break
					}
				}
				if coast {
					c.Biome = BiomeCoast
				} else {
					c.Biome = BiomeOcean
				}
			}
			continue
		}
		// Land — beaches are recognized by ocean/coast adjacency, regardless
		// of exact elevation. Rivers/lakes don't make beaches.
		oceanNb := false
		for _, nb := range neighbors[i] {
			nc := cells[nb]
			if nc.Terrain == "water" && !nc.River && !nc.Lake {
				oceanNb = true
				break
			}
		}
		switch {
		case oceanNb:
			c.Biome = BiomeBeach
		case c.Elevation > 0.85:
			c.Biome = BiomePeak
		case c.Elevation > 0.65:
			c.Biome = BiomeMountain
		case c.Elevation > 0.40:
			c.Biome = BiomeHills
		default:
			c.Biome = BiomeGrassland
		}
	}
}
