package terrain

// catmullRom returns a densified curve through the given control points
// using uniform Catmull-Rom interpolation. samplesPerSegment controls how
// many interpolated points are emitted between each pair of originals.
//
// Endpoints are duplicated (clamp mode) so the curve starts exactly at
// points[0] and ends at points[len-1], which means the client can draw
// the returned slice as a direct polyline/spline without boundary fudging.
func catmullRom(points []Point, samplesPerSegment int) []Point {
	n := len(points)
	if n < 2 || samplesPerSegment < 1 {
		out := make([]Point, n)
		copy(out, points)
		return out
	}
	out := make([]Point, 0, (n-1)*samplesPerSegment+1)
	out = append(out, points[0])
	for i := 0; i < n-1; i++ {
		p1 := points[i]
		p2 := points[i+1]
		p0 := p1
		if i > 0 {
			p0 = points[i-1]
		}
		p3 := p2
		if i+2 < n {
			p3 = points[i+2]
		}
		for k := 1; k <= samplesPerSegment; k++ {
			t := float64(k) / float64(samplesPerSegment)
			out = append(out, catmullRomPoint(p0, p1, p2, p3, t))
		}
	}
	// Fix float drift at the very last sample: Catmull-Rom at t=1 should
	// land exactly on the final control point but picks up 1–2 ULP of
	// rounding. Clients rely on curve[end] == last control exactly.
	out[len(out)-1] = points[n-1]
	return out
}

func catmullRomPoint(p0, p1, p2, p3 Point, t float64) Point {
	t2 := t * t
	t3 := t2 * t
	// Uniform Catmull-Rom basis:
	// 0.5 * [2P1 + (-P0+P2)t + (2P0-5P1+4P2-P3)t² + (-P0+3P1-3P2+P3)t³]
	return Point{
		X: 0.5 * (2*p1.X +
			(-p0.X+p2.X)*t +
			(2*p0.X-5*p1.X+4*p2.X-p3.X)*t2 +
			(-p0.X+3*p1.X-3*p2.X+p3.X)*t3),
		Y: 0.5 * (2*p1.Y +
			(-p0.Y+p2.Y)*t +
			(2*p0.Y-5*p1.Y+4*p2.Y-p3.Y)*t2 +
			(-p0.Y+3*p1.Y-3*p2.Y+p3.Y)*t3),
	}
}

// cellCenters gathers the center points of the given cell IDs in order.
func cellCenters(cells []Cell, ids []int) []Point {
	out := make([]Point, len(ids))
	for i, id := range ids {
		out[i] = cells[id].Center
	}
	return out
}

// nearBorder reports whether p sits within margin of the map rectangle edge.
func nearBorder(p Point, w, h, margin float64) bool {
	return p.X <= margin || p.X >= w-margin ||
		p.Y <= margin || p.Y >= h-margin
}

// projectToBorder returns p snapped onto the nearest map border (perpendicular
// foot). Used to extend river/highway curves out to the map edge so the
// geometry visibly enters/exits the world.
func projectToBorder(p Point, w, h float64) Point {
	type opt struct{ x, y, d float64 }
	cands := [4]opt{
		{0, p.Y, p.X},
		{w, p.Y, w - p.X},
		{p.X, 0, p.Y},
		{p.X, h, h - p.Y},
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.d < best.d {
			best = c
		}
	}
	return Point{X: best.x, Y: best.y}
}

// extendAtBorder prepends a border projection before centers[0] if that cell
// sits near a map edge, and appends one after the last cell similarly.
// Disabled ends are controlled by prepend/append flags so rivers (source only)
// and highways (both ends) can share the same helper.
func extendAtBorder(centers []Point, w, h float64, prepend, appendEnd bool) []Point {
	const borderMargin = 40.0
	if len(centers) < 2 {
		return centers
	}
	out := make([]Point, 0, len(centers)+2)
	if prepend && nearBorder(centers[0], w, h, borderMargin) {
		out = append(out, projectToBorder(centers[0], w, h))
	}
	out = append(out, centers...)
	if appendEnd && nearBorder(centers[len(centers)-1], w, h, borderMargin) {
		out = append(out, projectToBorder(centers[len(centers)-1], w, h))
	}
	return out
}
