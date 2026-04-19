package terrain

import "math/rand"

var lakeSizeMap = map[string]int{
	"small":  3,
	"medium": 7,
	"large":  15,
}

// generateLakes BFS-expands lake clusters on land cells.
func generateLakes(cells []Cell, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []Lake {
	targetSize := lakeSizeMap[cfg.Terrain.LakeSize]
	if targetSize == 0 {
		targetSize = 7
	}

	// Build a quick lookup: is cell already water?
	isWater := func(id int) bool { return cells[id].Terrain == "water" }

	var lakes []Lake
	usedInLake := make(map[int]bool)

	// Candidates: land cells not adjacent to existing water.
	var candidates []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
		tooClose := false
		for _, nb := range diag.neighbors[c.ID] {
			if isWater(nb) {
				tooClose = true
				break
			}
		}
		if !tooClose {
			candidates = append(candidates, c.ID)
		}
	}

	for id := 0; id < cfg.Terrain.LakeCount && len(candidates) > 0; id++ {
		// Pick random seed cell.
		idx := rng.Intn(len(candidates))
		seed := candidates[idx]
		candidates = append(candidates[:idx], candidates[idx+1:]...)
		if usedInLake[seed] {
			continue
		}

		// BFS expansion.
		cluster := []int{seed}
		queue := []int{seed}
		inCluster := map[int]bool{seed: true}

		for len(queue) > 0 && len(cluster) < targetSize {
			cur := queue[0]
			queue = queue[1:]
			for _, nb := range diag.neighbors[cur] {
				if inCluster[nb] || cells[nb].Terrain == "water" || usedInLake[nb] {
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

		// Mark cells as water.
		for _, cid := range cluster {
			cells[cid].Terrain = "water"
			usedInLake[cid] = true
		}

		lakes = append(lakes, Lake{
			ID:    id,
			Cells: cluster,
			Area:  len(cluster),
		})
	}
	return lakes
}
