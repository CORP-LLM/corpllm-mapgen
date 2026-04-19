package terrain

import (
	"math"
	"math/rand"
	"sort"
)

// voronoiDiagram holds raw diagram output before building Cells/Edges.
type voronoiDiagram struct {
	sites     []Point
	cellVerts [][]Point  // polygon vertices per site
	neighbors [][]int    // neighbor site indices per site
	edges     [][2]Point // shared edge endpoints per site pair
	edgePairs [][2]int   // site indices for each edge
}

// buildVoronoi generates a Voronoi diagram via brute-force clipping.
// Each cell polygon is computed by clipping a half-plane per neighbor.
func buildVoronoi(sites []Point, width, height float64) *voronoiDiagram {
	n := len(sites)
	d := &voronoiDiagram{
		sites:     sites,
		cellVerts: make([][]Point, n),
		neighbors: make([][]int, n),
	}

	// For each site, clip the bounding box against all perpendicular bisectors.
	bounds := []Point{
		{0, 0}, {width, 0}, {width, height}, {0, height},
	}
	for i, s := range sites {
		poly := make([]Point, len(bounds))
		copy(poly, bounds)
		for j, t := range sites {
			if i == j {
				continue
			}
			poly = clipPolygonByBisector(poly, s, t)
			if len(poly) == 0 {
				break
			}
		}
		d.cellVerts[i] = poly
	}

	// Build neighbor graph: two sites are neighbors if their cells share ≥2 coincident vertices.
	neighborSet := make([]map[int]bool, n)
	for i := range neighborSet {
		neighborSet[i] = make(map[int]bool)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			shared := sharedVertices(d.cellVerts[i], d.cellVerts[j])
			if len(shared) >= 2 {
				neighborSet[i][j] = true
				neighborSet[j][i] = true
				d.edgePairs = append(d.edgePairs, [2]int{i, j})
				d.edges = append(d.edges, [2]Point{shared[0], shared[1]})
			}
		}
	}
	for i := 0; i < n; i++ {
		for nb := range neighborSet[i] {
			d.neighbors[i] = append(d.neighbors[i], nb)
		}
		sort.Ints(d.neighbors[i])
	}
	return d
}

// clipPolygonByBisector clips poly by the half-plane containing site s
// relative to the perpendicular bisector between s and t.
func clipPolygonByBisector(poly []Point, s, t Point) []Point {
	if len(poly) == 0 {
		return poly
	}
	// Normal of bisector points from t toward s.
	nx := s.X - t.X
	ny := s.Y - t.Y
	// Mid-point on bisector.
	mx := (s.X + t.X) * 0.5
	my := (s.Y + t.Y) * 0.5
	// d: points with dot(p-m, n) >= 0 are on s's side.
	result := make([]Point, 0, len(poly)+1)
	for i, cur := range poly {
		next := poly[(i+1)%len(poly)]
		curIn := dot2(cur.X-mx, cur.Y-my, nx, ny) >= 0
		nextIn := dot2(next.X-mx, next.Y-my, nx, ny) >= 0
		if curIn {
			result = append(result, cur)
		}
		if curIn != nextIn {
			// Compute intersection of edge cur→next with bisector line.
			if pt, ok := lineIntersect(cur, next, Point{mx, my}, Point{mx + ny, my - nx}); ok {
				result = append(result, pt)
			}
		}
	}
	return result
}

func dot2(ax, ay, bx, by float64) float64 {
	return ax*bx + ay*by
}

func lineIntersect(a, b, c, d Point) (Point, bool) {
	// Parametric: a + t*(b-a) = c + u*(d-c)
	dxAB := b.X - a.X
	dyAB := b.Y - a.Y
	dxCD := d.X - c.X
	dyCD := d.Y - c.Y
	denom := dxAB*dyCD - dyAB*dxCD
	if math.Abs(denom) < 1e-10 {
		return Point{}, false
	}
	t := ((c.X-a.X)*dyCD - (c.Y-a.Y)*dxCD) / denom
	return Point{a.X + t*dxAB, a.Y + t*dyAB}, true
}

func sharedVertices(a, b []Point) []Point {
	const eps = 0.5
	var shared []Point
	for _, va := range a {
		for _, vb := range b {
			if math.Abs(va.X-vb.X) < eps && math.Abs(va.Y-vb.Y) < eps {
				shared = append(shared, va)
				break
			}
		}
	}
	return shared
}

// lloydRelax runs one Lloyd relaxation pass: move each site to its cell centroid.
func lloydRelax(sites []Point, width, height float64) []Point {
	d := buildVoronoi(sites, width, height)
	relaxed := make([]Point, len(sites))
	for i, poly := range d.cellVerts {
		if len(poly) == 0 {
			relaxed[i] = sites[i]
			continue
		}
		cx, cy := 0.0, 0.0
		for _, v := range poly {
			cx += v.X
			cy += v.Y
		}
		relaxed[i] = Point{cx / float64(len(poly)), cy / float64(len(poly))}
	}
	return relaxed
}

// generateSites creates n random points within bounds using rng.
func generateSites(n int, width, height float64, rng *rand.Rand) []Point {
	pts := make([]Point, n)
	for i := range pts {
		pts[i] = Point{rng.Float64() * width, rng.Float64() * height}
	}
	return pts
}
