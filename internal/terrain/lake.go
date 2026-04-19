package terrain

import "math/rand"

var lakeSizeMap = map[string]int{
	"small":  3,
	"medium": 7,
	"large":  15,
}

// generateLakes creates one lake per LakeSpec, each with its own size.
func generateLakes(cells []Cell, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []Lake {
	// Build a fresh candidate list (land cells not adjacent to water).
	seedCandidates := func() []int {
		var out []int
		for _, c := range cells {
			if c.Terrain != "land" {
				continue
			}
			tooClose := false
			for _, nb := range diag.neighbors[c.ID] {
				if cells[nb].Terrain == "water" {
					tooClose = true
					break
				}
			}
			if !tooClose {
				out = append(out, c.ID)
			}
		}
		return out
	}

	var lakes []Lake

	for id, spec := range cfg.Terrain.Lakes {
		targetSize := lakeSizeMap[spec.Size]
		if targetSize == 0 {
			targetSize = 7
		}

		candidates := seedCandidates() // refresh — previous lake may have changed the map

		// Pick the first valid seed.
		var seed int = -1
		for len(candidates) > 0 {
			idx := rng.Intn(len(candidates))
			cand := candidates[idx]
			candidates = append(candidates[:idx], candidates[idx+1:]...)
			if cells[cand].Terrain != "land" {
				continue
			}
			stillValid := true
			for _, nb := range diag.neighbors[cand] {
				if cells[nb].Terrain == "water" {
					stillValid = false
					break
				}
			}
			if stillValid {
				seed = cand
				break
			}
		}
		if seed == -1 {
			continue
		}

		cluster := []int{seed}
		queue := []int{seed}
		inCluster := map[int]bool{seed: true}

		for len(queue) > 0 && len(cluster) < targetSize {
			cur := queue[0]
			queue = queue[1:]
			for _, nb := range diag.neighbors[cur] {
				if inCluster[nb] || cells[nb].Terrain == "water" {
					continue
				}
				touchesOuterWater := false
				for _, nnb := range diag.neighbors[nb] {
					if !inCluster[nnb] && cells[nnb].Terrain == "water" {
						touchesOuterWater = true
						break
					}
				}
				if touchesOuterWater {
					continue
				}
				cluster = append(cluster, nb)
				inCluster[nb] = true
				queue = append(queue, nb)
				if len(cluster) >= targetSize {
					break
				}
			}
		}

		for _, cid := range cluster {
			cells[cid].Terrain = "water"
			cells[cid].Lake = true
		}

		lakes = append(lakes, Lake{
			ID:    id,
			Cells: cluster,
			Area:  len(cluster),
		})
	}
	return lakes
}
