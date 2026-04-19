package terrain

import (
	"math"
	"math/rand"
)

// generateRivers routes one river per RiverSpec. End determines the hydrological
// terminus: coast (ocean), lake, or offmap (map edge). Rivers never stop on land.
func generateRivers(cells []Cell, edges []Edge, diag *voronoiDiagram, cfg *Config, rng *rand.Rand) []River {
	w := float64(cfg.Width)
	h := float64(cfg.Height)
	borderDist := math.Min(w, h) * 0.15
	inlandDist := math.Min(w, h) * 0.3

	// Classify existing water for end-routing disambiguation.
	isCoastWater := func(c *Cell) bool { return c.Terrain == "water" && !c.Lake && !c.River }
	isLakeWater := func(c *Cell) bool { return c.Terrain == "water" && c.Lake }

	// Collect lake cell IDs once — used for end="lake" targeting.
	var lakeCellIDs []int
	for _, c := range cells {
		if c.Lake {
			lakeCellIDs = append(lakeCellIDs, c.ID)
		}
	}

	// Edge lookup.
	edgeIndex := make(map[[2]int]int, len(diag.edgePairs))
	for i, p := range diag.edgePairs {
		a, b := p[0], p[1]
		if a > b {
			a, b = b, a
		}
		edgeIndex[[2]int{a, b}] = i
	}

	// Partition land cells by zone once.
	// Border sources represent upstream inflow — the river enters the map
	// from beyond. Exclude cells on the coast-side border (those would be
	// trivial coastal rills right next to the sea, not rivers from beyond).
	isCoastSideBorder := func(cx, cy float64) bool {
		if !cfg.Terrain.CoastEnabled {
			return false
		}
		switch cfg.Terrain.CoastSide {
		case "north":
			return cy < borderDist
		case "south":
			return cy > h-borderDist
		case "east":
			return cx > w-borderDist
		case "west":
			return cx < borderDist
		}
		return false
	}
	hasWaterNeighbor := func(id int) bool {
		for _, nb := range diag.neighbors[id] {
			if cells[nb].Terrain == "water" {
				return true
			}
		}
		return false
	}
	var borderPool, inlandPool []int
	for _, c := range cells {
		if c.Terrain != "land" {
			continue
		}
		cx, cy := c.Center.X, c.Center.Y
		atBorder := cx < borderDist || cx > w-borderDist || cy < borderDist || cy > h-borderDist
		// Skip border cells already touching water — they produce 2-cell
		// "rivers" that dump straight into the sea.
		if atBorder && !isCoastSideBorder(cx, cy) && !hasWaterNeighbor(c.ID) {
			borderPool = append(borderPool, c.ID)
		}
		if cx >= inlandDist && cx <= w-inlandDist && cy >= inlandDist && cy <= h-inlandDist {
			inlandPool = append(inlandPool, c.ID)
		}
	}

	// coast-side unit vector (used by straightness bias for end=coast/offmap).
	var coastX, coastY float64
	switch cfg.Terrain.CoastSide {
	case "north":
		coastY = -1
	case "south":
		coastY = 1
	case "east":
		coastX = 1
	case "west":
		coastX = -1
	}

	var rivers []River
	used := make(map[int]bool)

	for id, spec := range cfg.Terrain.Rivers {
		// Skip silently if the spec asks for a lake but no lake exists.
		if spec.End == "lake" && len(lakeCellIDs) == 0 {
			continue
		}

		// Pool by origin.
		pool := borderPool
		if spec.Origin == "inland" {
			pool = inlandPool
		}
		srcID := pickSource(pool, cells, used, rng)
		if srcID == -1 {
			continue
		}
		srcCenter := cells[srcID].Center

		// Target direction for straightness bias.
		var tx, ty float64
		switch spec.End {
		case "coast":
			tx, ty = coastX, coastY
		case "offmap":
			// Away from coast (or toward the farthest edge from source).
			tx, ty = -coastX, -coastY
			if tx == 0 && ty == 0 {
				// No coast configured — aim for the nearest map edge.
				tx, ty = toNearestEdge(srcCenter, w, h)
			}
		case "lake":
			// Aim at the closest lake cell.
			target := nearestCellCenter(srcCenter, lakeCellIDs, cells)
			dx, dy := target.X-srcCenter.X, target.Y-srcCenter.Y
			l := math.Hypot(dx, dy)
			if l > 0 {
				tx, ty = dx/l, dy/l
			}
		}

		path := routeRiver(srcID, cells, diag.neighbors, w, h, used, spec, tx, ty,
			isCoastWater, isLakeWater, lakeCellIDs, rng)
		// Require a meaningful path — shorter than 5 cells is a coastal
		// rill, not a river worth rendering.
		if len(path) < 5 {
			continue
		}

		// Require hydrologically valid termination: water of any kind,
		// or for offmap, the map border.
		lastCell := &cells[path[len(path)-1]]
		validTerm := lastCell.Terrain == "water" ||
			(spec.End == "offmap" && cellTouchesBorder(lastCell, w, h))
		if !validTerm {
			continue
		}

		// Assemble edge path.
		edgePath := make([]int, 0, len(path)-1)
		for k := 0; k < len(path)-1; k++ {
			a, b := path[k], path[k+1]
			if a > b {
				a, b = b, a
			}
			if ei, ok := edgeIndex[[2]int{a, b}]; ok {
				edgePath = append(edgePath, ei)
				edges[ei].River = true
			}
		}
		if len(edgePath) == 0 {
			continue
		}

		for _, cid := range path {
			// Don't convert lake or coast cells we're flowing into.
			if cells[cid].Terrain == "water" {
				continue
			}
			cells[cid].River = true
			cells[cid].Terrain = "water"
			used[cid] = true
		}

		width := spec.Width
		if width == "" {
			width = "medium"
		}
		rivers = append(rivers, River{
			ID:       id,
			Path:     edgePath,
			CellPath: append([]int(nil), path...),
			Source:   cells[path[0]].Center,
			Mouth:    cells[path[len(path)-1]].Center,
			Width:    width,
		})
	}
	return rivers
}

func pickSource(pool []int, cells []Cell, used map[int]bool, rng *rand.Rand) int {
	local := append([]int(nil), pool...)
	for len(local) > 0 {
		idx := rng.Intn(len(local))
		cand := local[idx]
		local = append(local[:idx], local[idx+1:]...)
		if used[cand] || cells[cand].Terrain != "land" {
			continue
		}
		return cand
	}
	return -1
}

func nearestCellCenter(from Point, ids []int, cells []Cell) Point {
	best := cells[ids[0]].Center
	bestD := math.Hypot(best.X-from.X, best.Y-from.Y)
	for _, id := range ids[1:] {
		c := cells[id].Center
		d := math.Hypot(c.X-from.X, c.Y-from.Y)
		if d < bestD {
			best = c
			bestD = d
		}
	}
	return best
}

func toNearestEdge(p Point, w, h float64) (float64, float64) {
	// Return unit vector pointing to the nearest map edge.
	dists := []struct {
		dx, dy, d float64
	}{
		{-1, 0, p.X},    // west
		{1, 0, w - p.X}, // east
		{0, -1, p.Y},    // north
		{0, 1, h - p.Y}, // south
	}
	best := dists[0]
	for _, d := range dists[1:] {
		if d.d < best.d {
			best = d
		}
	}
	return best.dx, best.dy
}

func cellTouchesBorder(c *Cell, w, h float64) bool {
	const margin = 1.5
	for _, v := range c.Vertices {
		if v.X <= margin || v.X >= w-margin || v.Y <= margin || v.Y >= h-margin {
			return true
		}
	}
	return false
}

func reachedValidEnd(c *Cell, end string, w, h float64, isCoast, isLake func(*Cell) bool) bool {
	switch end {
	case "coast":
		return isCoast(c) || c.River // any river-connected water counts
	case "lake":
		return isLake(c)
	case "offmap":
		return cellTouchesBorder(c, w, h)
	}
	return false
}

// routeRiver greedily routes until it reaches a valid hydrological endpoint.
func routeRiver(srcID int, cells []Cell, neighbors [][]int, w, h float64,
	used map[int]bool, spec RiverSpec, tx, ty float64,
	isCoast, isLake func(*Cell) bool, lakeIDs []int,
	rng *rand.Rand) []int {

	const maxSteps = 250
	path := []int{srcID}
	visited := map[int]bool{srcID: true}
	cur := srcID

	maxDim := math.Max(w, h)
	// Scoring weights tuned to be comparable to the elevation term.
	// With dElev per step ≈ 0.02–0.1, elevScale (=3000) gives step-score ≈ 60–300.
	// Straightness and meander need similar magnitudes to actually affect choice.
	const elevScale = 3000.0
	const uphillExtra = 6000.0
	// At max straightness, alignWeight must dominate the elevation term so
	// concrete canals actually look concrete — humans dig through terrain.
	alignWeight := spec.Straightness * maxDim * 3.0 // up to ~3000 at max straight
	meanderWeight := spec.Meander * 500             // up to ±500 random swing

	for step := 0; step < maxSteps; step++ {
		c := &cells[cur]

		// Termination — rivers always end at water or, for offmap, the map edge.
		if step >= 1 {
			if c.Terrain == "water" {
				break
			}
			if spec.End == "offmap" && cellTouchesBorder(c, w, h) {
				break
			}
		}

		best := -1
		bestScore := math.MaxFloat64
		curElev := c.Elevation
		curCenter := c.Center

		// Primary rule: water flows downhill. Heavily penalize any uphill move
		// so rivers only climb if no downhill neighbor exists (last resort).
		for _, nb := range neighbors[cur] {
			if visited[nb] {
				continue
			}
			nc := &cells[nb]

			// Base score: elevation difference (negative = downhill = better).
			dElev := nc.Elevation - curElev
			score := dElev * elevScale
			// Uphill penalty compounds — strongly discouraged.
			if dElev > 0 {
				score += dElev * uphillExtra
			}

			// End-type preferences modulate the downhill choice. Tributary
			// merging requires the candidate to be at least 20% of the map
			// diagonal from the source — otherwise a second river would
			// merge on step 1 and never develop its own path.
			srcC := cells[srcID].Center
			dFromSrc := math.Hypot(nc.Center.X-srcC.X, nc.Center.Y-srcC.Y)
			score += endPreferenceScore(nc, spec.End, w, h, isCoast, isLake, lakeIDs, cells, dFromSrc, maxDim*0.20)

			if used[nb] {
				// Light penalty — enough that rivers don't pointlessly overlap,
				// but small enough that downhill routing isn't blocked when
				// the only viable path borders an existing river.
				score += 50
			}
			if alignWeight > 0 {
				mx := nc.Center.X - curCenter.X
				my := nc.Center.Y - curCenter.Y
				ml := math.Hypot(mx, my)
				if ml > 0 {
					align := (mx*tx + my*ty) / ml
					score -= align * alignWeight
				}
			}
			if meanderWeight > 0 {
				score += (rng.Float64()*2 - 1) * meanderWeight
			}

			if score < bestScore {
				bestScore = score
				best = nb
			}
		}

		// No valid neighbor at all → river ends here.
		if best == -1 {
			break
		}
		// If every candidate was uphill, we're in a local minimum — terminate
		// rather than climb. Real rivers pool up in basins (endorheic lakes).
		if cells[best].Elevation > curElev+0.01 && step >= 1 {
			break
		}

		path = append(path, best)
		visited[best] = true
		cur = best
	}
	return path
}

// endPreferenceScore adds a bias on top of the downhill score so rivers
// prefer the configured terminus when multiple downhill neighbors exist.
// dFromSource = distance from this candidate to the river's source;
// minTributaryDist = threshold below which tributary merges are disabled
// (prevents a new river from instantly snapping onto an existing one).
func endPreferenceScore(nc *Cell, end string, w, h float64,
	isCoast, isLake func(*Cell) bool, lakeIDs []int, cells []Cell,
	dFromSource, minTributaryDist float64) float64 {

	// Any existing river cell can be a tributary terminus — but only once
	// the candidate is far enough from this river's source. Otherwise a
	// second river would merge on step 1 and never develop its own path.
	const tributaryBonus = -3e8
	allowTributary := dFromSource >= minTributaryDist

	switch end {
	case "coast":
		if isCoast(nc) {
			return -1e9
		}
		if nc.River && allowTributary {
			return tributaryBonus
		}
		if isLake(nc) {
			return 5000 // prefer continuing to coast over ending in lake
		}
	case "lake":
		if isLake(nc) {
			return -1e9
		}
		if nc.River && allowTributary {
			return tributaryBonus
		}
		if isCoast(nc) {
			return 5000
		}
		if len(lakeIDs) > 0 {
			target := nearestCellCenter(nc.Center, lakeIDs, cells)
			return math.Hypot(target.X-nc.Center.X, target.Y-nc.Center.Y) * 5
		}
	case "offmap":
		if nc.Terrain == "water" {
			return 5000
		}
		if nc.River && allowTributary {
			return tributaryBonus
		}
		return distToEdge(nc.Center, w, h) * 3
	}
	return 0
}

func distToEdge(p Point, w, h float64) float64 {
	d := p.X
	if p.Y < d {
		d = p.Y
	}
	if w-p.X < d {
		d = w - p.X
	}
	if h-p.Y < d {
		d = h - p.Y
	}
	return d
}
